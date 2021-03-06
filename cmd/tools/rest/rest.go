// Copyright NetApp Inc, 2021 All rights reserved

package rest

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"goharvest2/pkg/conf"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	validShowArgs = []string{"apis", "params", "models", "data"}
)

type check struct {
	isValid bool
}

var args = &Args{}

type Args struct {
	Item          string
	Poller        string
	Api           string
	Config        string
	SwaggerPath   string
	Fields        string
	Field         []string
	QueryField    string
	QueryValue    string
	DownloadAll   bool
	MaxRecords    string
	ForceDownload bool
}

var Cmd = &cobra.Command{
	Use:   "rest",
	Short: "ONTAP Rest Utility",
	Long:  "ONTAP Rest Utility - Explore available ONTAP REST APIs",
}

var showCmd = &cobra.Command{
	Use:       "show",
	Short:     "item to show, one of: " + strings.Join(validShowArgs, ", "),
	Args:      cobra.ExactValidArgs(1),
	ValidArgs: validShowArgs,
	Run:       doShow,
}

func readOrDownloadSwagger() (string, error) {
	var (
		poller         *conf.Poller
		err            error
		addr           string
		shouldDownload = true
		swagTime       time.Time
	)

	if poller, addr, err = getPollerAndAddr(); err != nil {
		return "", err
	}

	tmp := os.TempDir()
	swaggerPath := filepath.Join(tmp, addr+"-swagger.yaml")
	fileInfo, err := os.Stat(swaggerPath)

	if os.IsNotExist(err) {
		fmt.Printf("%s does not exist downloading\n", swaggerPath)
	} else if !args.ForceDownload {
		swagTime = fileInfo.ModTime()
		twoWeeksAgo := swagTime.Local().AddDate(0, 0, -14)
		if swagTime.Before(twoWeeksAgo) {
			fmt.Printf("%s is more than two weeks old, re-download", swaggerPath)
		} else {
			shouldDownload = false
		}
	}
	if shouldDownload {
		url := "https://" + addr + "/docs/api/swagger.yaml"
		bytesDownloaded, err := downloadSwagger(poller, swaggerPath, url)
		if err != nil {
			fmt.Printf("error downloading swagger %s\n", err)
			return "", err
		}
		fmt.Printf("downloaded %d bytes from %s\n", bytesDownloaded, url)
	}
	fmt.Printf("Using downloaded file %s with timestamp %s\n", swaggerPath, swagTime)
	return swaggerPath, nil
}

func silentClose(body io.ReadCloser) {
	_ = body.Close()
}

func doShow(_ *cobra.Command, a []string) {
	c := validateArgs(a)
	if !c.isValid {
		return
	}
	if args.SwaggerPath != "" {
		doSwagger(*args)
	} else {
		doCmd()
	}
}

func validateArgs(strings []string) check {
	// One of Poller or SwaggerPath are allowed, but not both
	if args.Poller != "" && args.SwaggerPath != "" {
		fmt.Printf("Both poller and swagger are set. Only one or the other can be set, not both\n")
		return check{isValid: false}
	}
	if len(strings) == 0 {
		args.Item = ""
	} else {
		args.Item = strings[0]
	}
	qfSet := args.QueryField != ""
	qvSet := args.QueryValue != ""
	if args.Item == "data" && (qfSet != qvSet) {
		fmt.Printf(`Both "query-fields" and "query-value" must be specified if either is specified.` + "\n")
		return check{isValid: false}
	}
	return check{isValid: true}
}

func doCmd() {
	switch args.Item {
	case "apis", "params", "models":
		swaggerPath, err := readOrDownloadSwagger()
		if err != nil {
			return // everything logged earlier
		}
		args.SwaggerPath = swaggerPath
		doSwagger(*args)
	case "data":
		doData()
	}
}

type Pagination struct {
	Records    []interface{} `json:"records"`
	NumRecords int           `json:"num_records"`
	Links      *struct {
		Next struct {
			Href string `json:"href"`
		} `json:"next"`
	} `json:"_links,omitempty"`
}

func doData() {
	var (
		poller *conf.Poller
		err    error
		client *Client
	)

	if poller, _, err = getPollerAndAddr(); err != nil {
		return
	}

	if client, err = New(poller); err != nil {
		fmt.Printf("error creating new client %+v\n", err)
		os.Exit(1)
	}

	// strip leading slash
	if strings.HasPrefix(args.Api, "/") {
		args.Api = args.Api[1:]
	}
	var records []interface{}
	fetchData(client, buildHref(), &records)

	all := Pagination{
		Records:    records,
		NumRecords: len(records),
	}
	pretty, err := json.MarshalIndent(all, "", " ")
	if err != nil {
		stderr("error marshalling json %+v\n", err)
		return
	}
	fmt.Printf("%s\n", pretty)
}

func getPollerAndAddr() (*conf.Poller, string, error) {
	var (
		poller *conf.Poller
		err    error
		addr   string
	)
	if poller, err = conf.GetPoller2(args.Config, args.Poller); err != nil {
		fmt.Printf("Poller named [%s] does not exist\n", args.Poller)
		return nil, "", err
	}
	if addr = value(poller.Addr, ""); addr == "" {
		fmt.Printf("Poller named [%s] does not have a valid addr=[%s]\n", args.Poller, addr)
		return nil, "", err
	}
	return poller, addr, nil
}

func buildHref() string {
	href := strings.Builder{}
	href.WriteString("api/")
	href.WriteString(args.Api)
	href.WriteString("?return_records=true")
	addArg(&href, "&fields=", args.Fields)
	for _, field := range args.Field {
		addArg(&href, "&", field)
	}
	addArg(&href, "&query_fields=", args.QueryField)
	addArg(&href, "&query=", args.QueryValue)
	addArg(&href, "&max_records=", args.MaxRecords)

	return href.String()
}

func addArg(href *strings.Builder, field string, value string) {
	if value == "" {
		return
	}
	href.WriteString(field)
	href.WriteString(value)
}

func fetchData(client *Client, href string, records *[]interface{}) {
	stderr("fetching href=[%s]\n", href)
	getRest, err := client.GetRest(href)
	if err != nil {
		stderr("error making request api=%s err=%+v\n", href, err)
		return
	} else {
		// extract returned records since paginated records need to be merged into a single list
		var page Pagination
		err := json.Unmarshal(getRest, &page)
		if err != nil {
			stderr("error unmarshalling json %+v\n", err)
			return
		}

		*records = append(*records, page.Records...)

		// If all results are desired and there is a next link, follow it
		if args.DownloadAll && page.Links != nil {
			nextLink := page.Links.Next.Href
			if nextLink != "" {
				if nextLink == href {
					// nextLink is same as previous link, no progress is being made, exit
					return
				}
				fetchData(client, nextLink, records)
			}
		}
	}
}

func stderr(format string, a ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, format, a...)
}

func init() {
	configPath, _ := conf.GetDefaultHarvestConfigPath()

	Cmd.AddCommand(showCmd)
	flags := Cmd.PersistentFlags()
	flags.StringVarP(&args.Poller, "poller", "p", "", "name of poller (cluster), as defined in your harvest config")
	flags.StringVarP(&args.SwaggerPath, "swagger", "s", "", "path to Swagger (OpenAPI) file to read from")
	flags.StringVar(&args.Config, "config", configPath, "harvest config file path")

	showFlags := showCmd.Flags()
	showFlags.StringVarP(&args.Api, "api", "a", "", "REST API PATTERN to show")
	showFlags.BoolVar(&args.DownloadAll, "all", false, "Collect all records by walking pagination links")
	showFlags.StringVarP(&args.MaxRecords, "max-records", "m", "", "Limit the number of records returned before providing pagination link")
	showFlags.BoolVar(&args.ForceDownload, "download", false, "Force download Swagger file instead of using local copy")
	showFlags.StringVarP(&args.Fields, "fields", "f", "*", "Fields to return in the response <field>[,...].")
	showFlags.StringArrayVar(&args.Field, "field", []string{}, "Query a field by value (can be specified multiple times.)\n"+
		`If the value contains query characters (*|,!<>..), it must be quoted to avoid their special meaning
    *         wildcard
    < > <= >= comparisons
    3..10     range
    !water    negation
    3|5       matching value in a list
    {} and "" escape special characters`)
	showFlags.StringVarP(&args.QueryField, "query-field", "q", "", "Search fields named <string>, matching rows where the value of the field selected by <string> matches <query-value>.\n"+
		"comma-delimited list of fields, or * to search across all fields.")
	showFlags.StringVarP(&args.QueryValue, "query-value", "u", "", "Pattern to search for in all fields specified by <query-fields>\n"+
		"same query characters as <field> apply (see above)")

	Cmd.SetUsageTemplate(Cmd.UsageTemplate() + `
Examples:
  harvest rest -p infinity show apis                                        Query cluster infinity for available APIs
  harvest rest -p infinity show params --api svm/svms                       Query cluster infinity for svm parameters. These query parameters are used
                                                                            to filter requests.
  harvest rest -p infinity show models --api svm/svms                       Query cluster infinity for svm models. These describe the REST response
                                                                            received when sending the svm/svms GET request.
  harvest rest -p infinity show data --api svm/svms --field "state=stopped" Query cluster infinity for stopped svms.

  harvest rest -p infinity show data --api storage/volumes \                Query cluster infinity for all volumes where  
      --field "space.physical_used_percent=>70" \                               physical_used_percent is > 70% and 
      --field "space.total_footprint=>400G" \                                   total_footprint is > 400G 
      --fields "name,svm,space"                                             The response should contain name, svm, and space attributes of matching volumes.

  harvest rest -p infinity show data --api storage/volumes \                Query cluster infinity for all volumes where the name of any volume or child
      --query-field "name" --query-value "io_load|scale"                         resource matches io_load or scale.
`)
}

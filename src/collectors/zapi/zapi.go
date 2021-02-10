package main

import (
    "errors"
    "strings"
    "strconv"
    "sync"
    "time"

    "goharvest2/share/logger"
    "goharvest2/poller/struct/matrix"
    "goharvest2/poller/struct/options"
    "goharvest2/poller/struct/yaml"
    "goharvest2/poller/struct/xml"
    "goharvest2/poller/util"
    "goharvest2/poller/collector"

    client "goharvest2/poller/api/zapi"
)

type Zapi struct {
    *collector.AbstractCollector
    connection *client.Client
    system *client.System
    object string
	template_fn string
	template_type string
    instanceKeyPrefix []string
}

func New(name, obj string, options *options.Options, params *yaml.Node) collector.Collector {
    a := collector.New(name, obj, options, params)
    return &Zapi{AbstractCollector: a}
}


func (c *Zapi) Init() error {

    var err error

    Log = logger.New(c.Options.LogLevel, c.Name+":"+c.Object)
    
    if c.connection, err = client.New(c.Params); err != nil {
        //logger.Error(c.Prefix, "connecting: %v", err)
        return err
    }

    if c.system, err = c.connection.GetSystem(); err != nil {
        //logger.Error(c.Prefix, "system info: %v", err)
        return err
    }

    logger.Debug(c.Prefix, "Connected to: %s", c.system.String())

    template_fn := c.Params.GetChild("objects").GetChildValue(c.Object) // @TODO err handling

    template, err := collector.ImportSubTemplate(c.Options.Path, "default", template_fn, c.Name, c.system.Version)
    if err != nil {
        logger.Error(c.Prefix, "Error importing subtemplate: %s", err)
        return err
    }
    c.Params.Union(template, false)
 
    if err := c.InitAbc(); err != nil {
        return err
    }

    c.Data.SetGlobalLabel("system", c.system.Name)

    if expopt := c.Params.GetChild("export_options"); expopt != nil {
        c.Data.SetExportOptions(expopt)
    } else {
        return errors.New("missing export options")
    }

    c.Metadata.AddMetric("api_time", "api_time", true) // extra metric for measuring api time

    if c.object = c.Params.GetChildValue("object"); c.object == "" {
        logger.Warn(c.Prefix, "Missing object in template")
    }

    c.Data.Object = c.object
    c.Metadata.Object = c.object
    
    counters := c.Params.GetChild("counters")
    if counters == nil {
        logger.Warn(c.Prefix, "Missing counters in template")
    }

    if c.object == "" || counters == nil {
        return errors.New("missing parameters")
    }

    //counters.PrintTree(0)
    logger.Debug(c.Prefix, "Parsing counters: %d values, %d children", len(counters.Values), len(counters.Children))
    ParseCounters(c.Data, counters, make([]string, 0))
    logger.Debug(c.Prefix, "Built counter cache with %d Metrics and %d Labels", c.Data.MetricsIndex+1, len(c.Data.Instances))

    if len(c.Data.InstanceKeys) == 0 {
        logger.Error(c.Prefix, "No instance keys in template")
        return errors.New("invalid parameters")
    }

    old_prefix := ParseKeyPrefix(c.Data.GetInstanceKeys())
    c.instanceKeyPrefix = ParseShortestPath(c.Data)
    logger.Debug(c.Prefix, "Parsed Instance Keys: %v", c.Data.InstanceKeys)
    logger.Debug(c.Prefix, "Parsed Instance Key Prefix: %v", c.instanceKeyPrefix)

    logger.Debug(c.Prefix, "Old Instance Key Prefix: %v", old_prefix)

    return nil

}

func (c *Zapi) Start(wg *sync.WaitGroup) {

    defer wg.Done()

    for {

        c.Metadata.InitData()
        results := make([]*matrix.Matrix, 0)

        for _, task := range c.Schedule.GetTasks() {

            if c.Schedule.IsDue(task) {

                c.Schedule.Start(task)
                
                switch

                data, err := c.poll(task)

                if err != nil {
                    logger.Warn(c.Prefix, "%s poll failed: %v", task, err)
                    return
                }
                
                logger.Debug(c.Prefix, "%s poll completed", task)

                duration := c.Schedule.Stop(task)
                c.Metadata.SetValueSS("poll_time", task, duration.Seconds())
                
                if data != nil {
                    results = append(results, data)

                    logger.Debug(c.Prefix, "\n\nCOLLECTOR DATA\n")
                    //data.Print()

                    for _, p := range c.Plugins {
                        if p.GetType() == "data" {
                            logger.Debug(c.Prefix, "running plugin [%s]", p.GetName())

                            plugin_data, err := p.Run(data)
                            if err != nil {
                                Log.Error(err.Error())
                            } else if plugin_data != nil {
                                for _, x := range plugin_data {
                                    if x != nil {
                                        results = append(results, x)
                                        logger.Debug(c.Prefix, "\n\nPLUGIN DATA\n")
                                        //x.Print()
                                    }
                                }

                            }
                        }
                    }
                }
            }
        }

        logger.Debug(c.Prefix, "exporting session data (%d)", len(results))

        for _, e := range c.Exporters {
            if err := e.Export(c.Metadata); err != nil {
                logger.Warn(c.Prefix, "Metadata export to [%s] failed: %v", e.GetName(), err)
            } else {
                for _, data := range results {
                    if err := e.Export(data); err != nil {
                        logger.Warn(c.Prefix, "Data export to [%s] failed: %v", e.GetName(), err)
                    }
                }
            }
        }

        d := c.Schedule.SleepDuration()
        logger.Debug(c.Prefix, "Sleeping %s until next poll session", d.String())
        c.Schedule.Sleep()
    }
}

func (c *Zapi) poll(task string) (*matrix.Matrix, error) {
    switch task {
        case "data":
            return c.poll_data()
        case "instance":
            return nil, c.poll_instance()
        default:
            return nil, errors.New("invalid task: " + task)
    }
}

func (c *Zapi) poll_instance() error {
    var err error
    var root *xml.Node
    var instances []*xml.Node
    var old_count int
    var keys []string
    var keypaths [][]string
    var found bool

    logger.Debug(c.Prefix, "starting instance poll")

    start := time.Now()
    c.connection.BuildRequest(xml.New(c.Params.GetChildValue("query")))
    root, err = c.connection.Invoke()
    end := time.Since(start)

    c.Metadata.SetValueSS("api_time", "instance", end.Seconds())

    if err != nil {
        logger.Error(c.Prefix, "client request failed: %s", err)
        return err
    }

    old_count = len(c.Data.Instances)
    c.Data.ResetInstances()

    instances = xml.SearchByPath(root, c.instanceKeyPrefix)
    logger.Debug(c.Prefix, "Fetched %d instances!!!!", len(instances))
    keypaths = c.Data.GetInstanceKeys()

    logger.Debug(c.Prefix, "keys=%v keypaths=%v found=%v", keys, keypaths, found)

    count := 0

    for _, instance := range instances {
        //c.logger.Printf(c.Prefix, "Handling instance element <%v> [%s]", &instance, instance.GetName())
        keys, found = xml.SearchByNames(instance, c.instanceKeyPrefix, keypaths)
        logger.Debug(c.Prefix, "Fetched instance keys (%v): %s", keypaths, strings.Join(keys, "."))

        if !found {
            logger.Debug(c.Prefix, "Skipping instance, keys not found:")
            //xml.PrintTree(instance, 0)
        } else {
            _, err = c.Data.AddInstance(strings.Join(keys, "."))
            if err != nil {
                logger.Error(c.Prefix, "Error adding instance: %s", err)
            } else {
                logger.Debug(c.Prefix, "Added new Instance to cache [%s]", strings.Join(keys, "."))
                count += 1
            }
        }
        //xmltree.PrintTree(instance, 0)
        //break
    }

    c.Metadata.SetValueSS("count", "instance", float64(count))

    //c.data.PrintInstances()
    logger.Info(c.Prefix, "added %d instances to cache (old cache had %d)", count, old_count)

    if len(c.Data.Instances) == 0 {
        logger.Info(c.Prefix, "Enterying standby mode until instances are detected")
        c.Status = "standby"
        c.Message = "no instances"
        c.Schedule.SetStandbyMode("instance", 1 * time.Hour)
    } else if c.Schedule.IsStandbyMode() {
        logger.Info(c.Prefix, "Unsetting standby mode")
        c.Status = "up"
        c.Message = ""
        c.Schedule.UnsetStandbyMode()
    }

    return nil
}

func (c *Zapi) poll_data() (*matrix.Matrix, error) {
    var err error
    var query string
    var node *xml.Node
    var fetch func(*matrix.Instance, *xml.Node, []string)
    var count, skipped int

    count = 0
    skipped = 0

    fetch = func(instance *matrix.Instance, node *xml.Node, path []string) {
        newpath := append(path, node.GetName())
        key := strings.Join(newpath, ".")
        metric := c.Data.GetMetric(key)
        content, has := node.GetContent()

        if has {
            if metric != nil {
                if float, err := strconv.ParseFloat(string(content), 64); err != nil {
                    logger.Warn(c.Prefix, "%sSkipping metric [%s]: failed to parse [%s] float%s", util.Red, key, content, util.End)
                    skipped += 1
                } else {
                    c.Data.SetValue(metric, instance, float)
                    logger.Trace(c.Prefix, "%sMetric [%s] - Set Value [%f]%s", util.Green, key, float, util.End)
                    count += 1
                }
            } else if label, found := c.Data.GetLabel(key); found {
                //c.Data.SetInstanceLabel(instance, label, string(content))
                instance.Labels.Set(label, string(content))
                logger.Trace(c.Prefix, "%sMetric [%s] (%s) Set Value [%s] as Instance Label%s", util.Yellow, label, key, content, util.End)
                count += 1
            } else {
                logger.Trace(c.Prefix, "%sSkipped [%s]: not found in metric or label cache%s", util.Blue, key, util.End)
                skipped += 1
            }
        } else {
            logger.Trace(c.Prefix, "Skipping metric [%s] with no value", key)
            skipped += 1
        }

        for _, child := range node.GetChildren() {
            fetch(instance, child, newpath)
        }
    }

    logger.Debug(c.Prefix, "starting data poll")

    if err = c.Data.InitData(); err != nil {
        logger.Warn(c.Prefix, "Len metrics: %d", len(c.Data.Metrics))
        logger.Warn(c.Prefix, "Len instances: %d", len(c.Data.Instances))
        return nil, err
    }

    if query = c.Params.GetChildValue("query"); query == "" {
        return nil, errors.New("missing query in template")
    }

    c.connection.BuildRequest(xml.New(query))

    if node, err = c.connection.Invoke(); err != nil {
        logger.Debug(c.Prefix, "Request for [%s] failed: %s", query, err)
        return nil, err
    }

    instances := xml.SearchByPath(node, c.instanceKeyPrefix)
    logger.Debug(c.Prefix, "Fetched %d instance elements", len(instances))

    for _, instanceElem := range instances {
        //c.logger.Printf(c.Prefix, "Handling instance element <%v> [%s]", &instance, instance.GetName())
        keys, found := xml.SearchByNames(instanceElem, c.instanceKeyPrefix, c.Data.GetInstanceKeys())
        logger.Debug(c.Prefix, "Fetched instance keys: %s", strings.Join(keys, "."))

        if !found {
            logger.Debug(c.Prefix, "Skipping instance: no keys fetched")
            continue
        }

        instance := c.Data.GetInstance(strings.Join(keys, "."))

        if instance == nil {
            logger.Debug(c.Prefix, "Skipping instance [%s]: not found in cache", strings.Join(keys, "."))
            continue
        }
        //path := make([]string, 0)
        //copy(path, c.InstanceKeyPrefix)
        fetch(instance, instanceElem, make([]string, 0))
    }
    //xmltree.PrintTree(node, 0)

    return c.Data, nil
}

/*

func News(name string, options *opts.Opts, params *yaml.Node) ([]*Zapi, error) {
    var subcollectors []*Zapi
    var err error

    Log = logger.New(options.LogLevel, class)
    
    connection, err := client.New(params)
    if err != nil {
        logger.Error(c.Prefix, "connecting: %v", err)
        return, subcollectors, err
    }

    system_info, err := connection.GetSystemInfo()
    if err != nil {
        logger.Error(c.Prefix, "system info: %v", err)
        return subcollectors, err
    }

    template, err := abc.ImportTemplate(options.Path, name)
    if err != nil {
        logger.Error(c.Prefix, "load template: %v", err)
        return subcollectors, err
    }

    objects := template.GetChild("objects")
    if objects == nil {
        logger.Error(c.Prefix, "no objects in template")
        return subcollectors, errors.New("no objects in template")
    }

    params.Union(template, false)

    for _, object := range objects.GetChildren() {
        c := New(name, object, optionos, params.Copy())
        c.system = system_info
        logger.Debug(c.Prefix, "Initialized subcollector [%s:%s]", name, c.object)
        subcollectors = append(subcollectors, c)
    }

    return subcollectors
}
*/

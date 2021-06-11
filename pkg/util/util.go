/*
 * Copyright NetApp Inc, 2021 All rights reserved

 Package Description:
    Some helper methods.
*/
package util

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func MinLen(elements [][]string) int {
	var min, i int
	min = len(elements[0])
	for i = 1; i < len(elements); i += 1 {
		if len(elements[i]) < min {
			min = len(elements[i])
		}
	}
	return min
}

func MaxLen(elements [][]string) int {
	var max, i int
	max = len(elements[0])
	for i = 1; i < len(elements); i += 1 {
		if len(elements[i]) > max {
			max = len(elements[i])
		}
	}
	return max
}

func AllSame(elements [][]string, k int) bool {
	var i int
	for i = 1; i < len(elements); i += 1 {
		if elements[i][k] != elements[0][k] {
			return false
		}
	}
	return true
}

func EqualStringSlice(a, b []string) bool {
	var i int
	if len(a) != len(b) {
		return false
	}
	for i = 0; i < len(a); i += 1 {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func GetFreePort(addr string) (int, error) {
	url, err := net.ResolveTCPAddr("tcp", addr+":0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", url)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func CheckFileExists(filePath string) bool {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false
	}
	return true
}

func GetCmdLine(pid int) (string, error) {
	data, err := ioutil.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return "", err
	}
	result := string(bytes.ReplaceAll(data, []byte("\x00"), []byte(" ")))
	return result, nil
}

func RemoveEmptyStrings(s []string) []string {
	var r []string
	for _, str := range s {
		if str != "" {
			r = append(r, str)
		}
	}
	return r
}

func GetProcessPids(search string) ([]int, error) {
	var result []int
	var ee *exec.ExitError
	var pe *os.PathError
	data, err := exec.Command("pgrep", "-f", search).Output()
	if errors.As(err, &ee) {
		return result, nil // ran, but non-zero exit code
	} else if errors.As(err, &pe) {
		return result, err // "no such file ...", "permission denied" etc.
	} else if err != nil {
		return result, err // something really bad happened!
	}
	sdata := string(data)
	pids := RemoveEmptyStrings(strings.Split(sdata, "\n"))
	for _, pid := range pids {
		p, err := strconv.Atoi(strings.TrimSpace(pid))
		if err != nil {
			return result, err
		}
		result = append(result, p)
	}
	return result, err
}

func ContainsWholeWord(source string, search string) bool {
	if len(source) == 0 || len(search) == 0 {
		return false
	}
	fields := strings.Fields(source)
	for _, w := range fields {
		if w == search {
			return true
		}
	}
	return false
}

func LastString(ss []string) string {
	return ss[len(ss)-1]
}

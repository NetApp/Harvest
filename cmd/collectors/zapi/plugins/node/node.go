//
// Copyright NetApp Inc, 2021 All rights reserved
//
// Package Description:
//
// Examples:
//
package main

import (
    "goharvest2/cmd/poller/collector/plugin"
    "goharvest2/pkg/matrix"
    "strings"
)

type Node struct {
    *plugin.AbstractPlugin
}

func New(p *plugin.AbstractPlugin) plugin.Plugin {
    return &Node{AbstractPlugin: p}
}

func (p *Node) Run(data *matrix.Matrix) ([]*matrix.Matrix, error) {

    for _, instance := range data.GetInstances() {

        warnings := make([]string, 0)

        if w := instance.Labels.Get("failed_fan_message"); w != "" && w != "There are no failed fans." {
            warnings = append(warnings, w)
        }

        if w := instance.Labels.Get("failed_power_message"); w != "" && w != "There are no failed power supplies." {
            warnings = append(warnings, w)
        }

        instance.Labels.Set("warnings", strings.Join(warnings, " "))
    }

    return nil, nil
}
//go:build !linux

package main

import svc "github.com/kardianos/service"

func makeAgentServiceOptions() svc.KeyValue {
	return svc.KeyValue{
		"OnFailureDelayDuration": "10",
	}
}

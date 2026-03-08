//go:build !linux

package main

import svc "github.com/kardianos/service"

func makeServiceOptions() svc.KeyValue {
	return svc.KeyValue{
		"OnFailureDelayDuration": "5",
	}
}

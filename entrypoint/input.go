package main

import (
	"strings"

	"github.com/sethvargo/go-githubactions"
)

func GetArgsAsArray(args []string) []string {
	var value []string

	for _, s := range args {
		for _, ss := range strings.Split(s, ",") {
			ss = strings.TrimSpace(ss)
			if ss != "" {
				value = append(value, ss)
			}
		}
	}

	return value
}

func GetInputAsArray(i string) []string {
	var value []string

	for _, s := range strings.Split(githubactions.GetInput(i), "\n") {
		for _, ss := range strings.Split(s, ",") {
			ss = strings.TrimSpace(ss)
			if ss != "" {
				value = append(value, ss)
			}
		}
	}

	return value
}

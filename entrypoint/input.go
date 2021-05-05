package main

import (
	"strings"

	"github.com/sethvargo/go-githubactions"
)

func GetInputAsArray(i string) (value []string) {
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
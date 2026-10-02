package config

import "os"

func initWorkDir() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return wd
}

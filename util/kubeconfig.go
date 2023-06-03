package util

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

type KubeConfig struct {
	Clusters []Cluster `yaml:"clusters"`
}

type Cluster struct {
	Cluster    ClusterDetails `yaml:"cluster"`
}

type ClusterDetails struct {
	Server               string `yaml:"server"`
}

func GetServerIpFromYaml(filepath string) string {
	yamlFile, err := ioutil.ReadFile(filepath)
	if err != nil {
		fmt.Println("Failed to read yaml file:", err)
		os.Exit(1)
	}

	var cfg KubeConfig
	err = yaml.Unmarshal(yamlFile, &cfg)
	if err != nil {
		fmt.Println("Failed to parse yaml file:", err)
		os.Exit(1)
	}
	urlStr := cfg.Clusters[0].Cluster.Server
	ipPattern := `((?:\d{1,3}\.){3}\d{1,3})`
	regex := regexp.MustCompile(ipPattern)
	ip := regex.FindString(urlStr)
	return ip
}

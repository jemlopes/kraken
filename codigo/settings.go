package main

import (
	"log"

	"gopkg.in/yaml.v2"

	"io/ioutil"
)

//Settings ..
type settings struct {
	App struct {
		Address    string
		Port       string
		ApiVersion string `yaml:"apiVersion"`
	}
	Cassandra struct {
		ContactPoints         string `yaml:"contactPoints"`
		Port                  string
		Protoversion          int `yaml:"protoVersion"`
		Keyspace              string
		Connections           int
		EnableFullConsistency bool `yaml:"enableFullConsistency"`
	}
	Log struct {
		Level string `yaml:"level"`
		Path  string `yaml:"path"`
	}
	Cache struct {
		EnableCache                  bool   `yaml:"enableCache"`
		UseDistributed               bool   `yaml:"useDistributed"`
		DistributionPoints           string `yaml:"distributionPoints"`
		ExpirationTimeinMinutes      int    `yaml:"expirationTimeinMinutes"`
		UpdateIntervalinMillis       int    `yaml:"updateIntervalinMillis"`
		BroadCastTimeInMillis        int    `yaml:"broadCastTimeInMillis"`
		ConsistenceToleranceInMillis int    `yaml:"consistenceToleranceInMillis"`
		ItemLimit                    int    `yaml:"itemLimit"`
	}
}

//GetSettings ...
//Get funcion
func getSettings() settings {

	data, err := ioutil.ReadFile("kraken-config.yaml")
	if err != nil {
		log.Fatal(err)
	}

	t := settings{}

	err = yaml.Unmarshal([]byte(data), &t)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	return t
}

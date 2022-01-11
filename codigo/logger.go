package main

import (
	"os"
	"time"

	"github.com/Sirupsen/logrus"
)

var logc = logrus.New()

func StartLogging(currentLocation *time.Location) {
	//Default time format template: Mon Jan 2 15:04:05 MST 2006
	logfiletime := time.Now().In(currentLocation).Format("20060102_150405")

	level, _ := logrus.ParseLevel(config.Log.Level)
	tf := new(KTextFormatter)
	logc.Formatter = tf
	logc.Level = level

	file, err := os.Create(config.Log.Path + "/kraken_" + logfiletime + ".log")
	//file, _ := os.Create("./log.txt")
	
	if  err != nil {
			logc.Out = os.Stdout
			logc.Fatalf("Não foi possivel iniciar sistema de logs: %v", err)
	} else {
		logc.Out = file
	}



}

/*
import (
	"os"
	"time"

	"github.com/op/go-logging"
)

var logc = logging.MustGetLogger("KrakenLogger")

func StartLogging(currentLocation *time.Location) {
	//Default time format template: Mon Jan 2 15:04:05 MST 2006
	//logfiletime := time.Now().In(currentLocation).Format("20060102_150405")

	//file, _ := os.Create("./kraken_" + logfiletime + ".log")
	file, _ := os.Create("./kraken_.log.txt")
	loggingBackend := logging.NewLogBackend(file, "", 0)
	// Example format string. Everything except the message has a custom color
	// which is dependent on the log level. Many fields have a custom output
	// formatting too, eg. the time returns the hour down to the milli second.
	logformat := logging.MustStringFormatter(
		`%{time:15:04:05.000} %{shortfunc} > %{level:.5s} %{id:03x} %{message}`,
	)

	loggingBackendFormatted := logging.NewBackendFormatter(loggingBackend, logformat)

	loglevel, _ := logging.LogLevel(config.Log.Level)
	loggingBackendLeveled := logging.AddModuleLevel(loggingBackendFormatted)
	loggingBackendLeveled.SetLevel(loglevel, "")


	logging.SetBackend(loggingBackendFormatted)
}
*/

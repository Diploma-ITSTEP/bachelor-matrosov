package main

import (
	"flag"
	"github.com/gidra39/mlflow-autostop/config"
	"github.com/gidra39/mlflow-autostop/mlflow"
	"log"
)

func main() {
	configuration := config.LoadConfig(".env", "config.json", "config.yaml")
	runID := flag.String("run-id", "", "MLflow run ID to monitor (optional)")
	experimentID := flag.String("experiment-id", "", "MLflow experiment ID to monitor (optional)")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	if *debug {
		log.Println("Debug mode enabled - verbose logging activated")
	}

	log.Printf("Using MLflow tracking URI: %s", configuration.MLflowTrackingURI)

	if *runID != "" {
		log.Printf("Monitoring specific run ID: %s", *runID)
		mlflow.MonitorSpecificRun(*runID, configuration, *debug)
	} else if *experimentID != "" {
		log.Printf("Monitoring active runs in experiment ID: %s", *experimentID)
		mlflow.MonitorExperiment(*experimentID, configuration, *debug)
	} else {
		log.Println("Monitoring all active runs")
		mlflow.MonitorAllActiveRuns(configuration, *debug)
	}
}

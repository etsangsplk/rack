package workers

import (
	"math"
	"os"
	"time"

	"github.com/convox/logger"
	"github.com/convox/rack/api/models"
)

var (
	autoscale = (os.Getenv("AUTOSCALE") == "true")
	tick      = 1 * time.Minute
)

func StartAutoscale() {
	autoscaleRack()

	for range time.Tick(tick) {
		autoscaleRack()
	}
}

func autoscaleRack() {
	log := logger.New("ns=workers.autoscale").At("autoscaleRack")

	// do nothing unless autoscaling is on
	if !autoscale {
		return
	}

	capacity, err := models.Provider().CapacityGet()
	if err != nil {
		log.Error(err)
		return
	}

	system, err := models.Provider().SystemGet()
	if err != nil {
		log.Error(err)
		return
	}

	log.Logf("status=%q", system.Status)
	if system.Status != "running" {
		return
	}

	// start with the current count
	desired := 0

	// calculate instances required to statisfy cpu reservations plus one for breathing room
	if c := int(math.Ceil(float64(capacity.ProcessCPU)/float64(capacity.InstanceCPU))) + 1; c > desired {
		log = log.Replace("reason", "cpu")
		desired = c
	}

	// calculate instances required to statisfy memory reservations plus one for breathing room
	if c := int(math.Ceil(float64(capacity.ProcessMemory)/float64(capacity.InstanceMemory))) + 1; c > desired {
		log = log.Replace("reason", "memory")
		desired = c
	}

	// instance count cant be less than 2
	if desired < 2 {
		log = log.Replace("reason", "minimum")
		desired = 2
	}

	// instance count must be at least maxconcurrency+1
	if c := int(capacity.ProcessWidth) + 1; c > desired {
		log = log.Replace("reason", "width")
		desired = c
	}

	// if no change then exit
	if system.Count == desired {
		return
	}

	log.Logf("change=%d", (desired - system.Count))

	// ok to start multiple but shut them down one at a time
	if desired < system.Count {
		system.Count--
	} else {
		system.Count = desired
	}

	err = models.Provider().SystemSave(*system)
	if err != nil {
		log.Error(err)
		return
	}
}

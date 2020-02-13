package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/tyk/config"
	"github.com/TykTechnologies/tyk/storage"
	"github.com/sirupsen/logrus"
)

type (
	HealthCheckStatus string

	HealthCheckComponentType string
)

const (
	Pass HealthCheckStatus = "pass"
	Fail                   = "fail"
	Warn                   = "warn"

	Component HealthCheckComponentType = "component"
	Datastore                          = "datastore"
	System                             = "system"
)

var (
	healthCheckInfo atomic.Value
	healthCheckLock sync.Mutex
)

func setCurrentHealthCheckInfo(h map[string]HealthCheckItem) {
	healthCheckLock.Lock()
	healthCheckInfo.Store(h)
	healthCheckLock.Unlock()
}

func getHealthCheckInfo() map[string]HealthCheckItem {
	healthCheckLock.Lock()
	ret := healthCheckInfo.Load().(map[string]HealthCheckItem)
	healthCheckLock.Unlock()
	return ret
}

type HealthCheckResponse struct {
	Status      HealthCheckStatus          `json:"status"`
	Version     string                     `json:"version,omitempty"`
	Output      string                     `json:"output,omitempty"`
	Description string                     `json:"description,omitempty"`
	Details     map[string]HealthCheckItem `json:"details,omitempty"`
}

type HealthCheckItem struct {
	Status        HealthCheckStatus `json:"status"`
	Output        string            `json:"output,omitempty"`
	ComponentType string            `json:"componentType,omitempty"`
	ComponentID   string            `json:"componentId,omitempty"`
	Time          string            `json:"time"`
}

func initHealthCheck(ctx context.Context) {
	setCurrentHealthCheckInfo(make(map[string]HealthCheckItem, 3))

	go func(ctx context.Context) {
		var n = config.Global().LivenessCheck.CheckDuration

		if n == 0 {
			n = 10
		}

		ticker := time.NewTicker(time.Second * n)

		for {
			select {
			case <-ctx.Done():

				ticker.Stop()
				mainLog.WithFields(logrus.Fields{
					"prefix": "health-check",
				}).Debug("Stopping Health checks for all components")
				return

			case <-ticker.C:
				gatherHealthChecks()
			}
		}
	}(ctx)
}

func gatherHealthChecks() {

	allInfos := make(map[string]HealthCheckItem, 3)

	redisStore := storage.RedisCluster{KeyPrefix: "livenesscheck-"}

	key := "tyk-liveness-probe"

	var wg sync.WaitGroup

	go func() {

		wg.Add(1)

		defer wg.Done()

		var checkItem = HealthCheckItem{
			Status:        Pass,
			ComponentType: Datastore,
			Time:          time.Now().Format(time.RFC3339),
		}

		err := redisStore.SetRawKey(key, key, 10)
		if err != nil {
			mainLog.WithField("liveness-check", true).WithError(err).Error("Redis health check failed")
			checkItem.Output = err.Error()
			checkItem.Status = Fail
		}

		allInfos["redis"] = checkItem
	}()

	if config.Global().UseDBAppConfigs {

		wg.Add(1)

		go func() {
			defer wg.Done()

			var checkItem = HealthCheckItem{
				Status:        Pass,
				ComponentType: Datastore,
				Time:          time.Now().Format(time.RFC3339),
			}

			if err := DashService.Ping(); err != nil {
				mainLog.WithField("liveness-check", true).Error(err)
				checkItem.Output = err.Error()
				checkItem.Status = Fail
			}

			checkItem.ComponentType = System
			allInfos["dashboard"] = checkItem
		}()
	}

	if config.Global().Policies.PolicySource == "rpc" {

		wg.Add(1)

		go func() {
			defer wg.Done()

			var checkItem = HealthCheckItem{
				Status:        Pass,
				ComponentType: Datastore,
				Time:          time.Now().Format(time.RFC3339),
			}

			rpcStore := RPCStorageHandler{KeyPrefix: "livenesscheck-"}

			if !rpcStore.Connect() {
				checkItem.Output = "Could not connect to RPC"
				checkItem.Status = Fail
			}

			checkItem.ComponentType = System

			allInfos["rpc"] = checkItem
		}()
	}

	wg.Wait()

	setCurrentHealthCheckInfo(allInfos)
}

func liveCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		doJSONWrite(w, http.StatusMethodNotAllowed, apiError(http.StatusText(http.StatusMethodNotAllowed)))
		return
	}

	checks := getHealthCheckInfo()

	res := HealthCheckResponse{
		Status:      Pass,
		Version:     VERSION,
		Description: "Tyk GW",
		Details:     checks,
	}

	var failCount int

	for _, v := range checks {
		if v.Status == Fail {
			failCount++
		}
	}

	var status HealthCheckStatus

	switch failCount {
	case 0:
		status = Pass

	case len(checks):
		status = Fail

	default:
		status = Warn
	}

	res.Status = status

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(res)
}

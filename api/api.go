package api

import (
	"fmt"
	"github.com/imroc/req"
	"github.com/meklis/all-ok-radius-server/api/cache"
	"github.com/meklis/all-ok-radius-server/api/sources"
	"github.com/meklis/all-ok-radius-server/logger"
	"github.com/meklis/all-ok-radius-server/prom"
	"github.com/meklis/all-ok-radius-server/radius/events"
	"github.com/ztrue/tracerr"
	"sync"
	"time"
)

type Api struct {
	sync.Mutex
	Conf    ApiConfig
	cache   *cache.CacheApi
	sources *sources.Sources
	lg      *logger.Logger
}

func Init(conf ApiConfig, lg *logger.Logger) *Api {
	api := new(Api)
	api.Conf = conf
	api.cache = cache.Init(conf.RadReply.Caching.TimeoutExpires)
	api.sources = sources.New(conf.RadReply.Addresses, lg, conf.RadReply.AliveChecking.DisableTimeout)
	api.lg = lg
	return api
}

func (a *Api) Get(req *events.Request) (*events.Response, error) {
	hash := req.GetHash()
	response := new(events.Response)
	response, exist := a.cache.Get(hash)
	if exist {
		a.lg.DebugF("%v found in cache, check actual time", hash)
		if response.Time.After(time.Now()) {
			a.lg.DebugF("%v has actual time - %v, returning from cache", hash, response.Time.String())
			return response, nil
		} else {
			a.lg.DebugF("%v must be actualized from api", hash)
		}
	}
	a.lg.DebugF("%v try get data over API", hash)

	apiResp, err := a._getFromApi(req)
	if err != nil && exist {
		prom.ErrorsInc(prom.Error, "api")
		a.lg.ErrorF("error get data from api: %v", tracerr.Sprint(err))
		return response, nil
	} else if err != nil {
		return nil, tracerr.Wrap(err)
	}
	actualizeTime := time.Now().Add(a.Conf.RadReply.Caching.ActualizeTimeout)
	if actualizeTime.After(time.Now().Add(time.Second * time.Duration(apiResp.LeaseTimeSec))) {
		a.lg.Warningf("Detected lease_time_sec has a small time. Actualize time will be set as lease time")
		actualizeTime = time.Now().Add(time.Second * time.Duration(apiResp.LeaseTimeSec))
	}
	apiResp.Time = actualizeTime
	a.cache.Set(hash, *apiResp)
	return apiResp, nil
}
func (a *Api) SendPostAuth(auth PostAuth) {
	if !a.Conf.PostAuth.Enabled {
		return
	}
	go func() {
		for _, addr := range a.Conf.PostAuth.Addresses {
			response, err := req.Post(addr, req.BodyJSON(&auth))
			if err != nil {
				prom.ErrorsInc(prom.Error, "api")
				a.lg.ErrorF("Post auth report returned err from addr %v: %v", addr, tracerr.Sprint(err))
				continue
			}
			if response.Response().StatusCode != 200 {
				prom.ErrorsInc(prom.Error, "api")
				a.lg.ErrorF("Post auth report returned err from addr %v: %v", addr, tracerr.Sprint(err))
				continue
			}
		}
	}()
}

func (a *Api) _getFromApi(request *events.Request) (*events.Response, error) {
	source, err := a.sources.GetSource()
	if err != nil {
		a.lg.DebugF("not found sources - %v", err.Error())
		return nil, tracerr.Wrap(err)
	}
	a.lg.DebugF("defined source from rr = %v", source)
	a.sources.IncRequests(source.Address)
	req.SetTimeout(a.Conf.Timeout)
	response, err := req.Post(source.Address, req.BodyJSON(request))
	if err != nil {
		prom.ErrorsInc(prom.Error, "api")
		a.lg.ErrorF("source returned err: %v", tracerr.Sprint(err))
		a.sources.Disable(source.Address)
		return nil, tracerr.Wrap(err)
	}
	if response.Response().StatusCode != 200 {
		prom.ErrorsInc(prom.Error, "api")
		a.lg.ErrorF("source returned http != 200: %v %v", response.Response().StatusCode, response.Response().Status)
		a.sources.Disable(source.Address)
		return nil, tracerr.New(fmt.Sprintf("http err: %v - %v", response.Response().StatusCode, response.Response().Status))
	}
	apiResp := ApiResponse{}
	if err := response.ToJSON(&apiResp); err != nil {
		a.sources.Disable(source.Address)
		return nil, tracerr.Wrap(err)
	}

	if apiResp.StatusCode != 200 {
		return nil, tracerr.New(fmt.Sprintf("api returned status code - %v. must be 200", apiResp.StatusCode))
	}
	return &apiResp.Data, nil
}

package internal

import (
	"fmt"
	"github.com/Kong/go-pdk"
	"github.com/go-playground/validator/v10"
	"github.com/mitchellh/hashstructure/v2"
	"strconv"
)

type CacheKey struct {
	Consumer  string
	Service   string
	Route     string
	Method    string `validate:"required" `
	URL       string `validate:"required" `
	QueryArgs map[string][]string
	Headers   map[string]string
	Body      []byte
	CacheTTL  int
}

func (c *CacheKey) String() string {
	return fmt.Sprintf(
		"CacheKey{Consumer: %s, Service: %s, Route: %s, Method: %s, URL: %s, QueryArgs: %v, Headers: %v, BodyLen: %d, CacheTTL: %d}",
		c.Consumer, c.Service, c.Route, c.Method, c.URL, c.QueryArgs, c.Headers, len(c.Body), c.CacheTTL,
	)
}

func consumerID(kong *pdk.PDK) (string, error) {
	obj, err := kong.Client.GetConsumer()
	if err != nil {
		return "", err
	}
	return obj.Id, nil
}

func serviceID(kong *pdk.PDK) (string, error) {
	obj, err := kong.Router.GetService()
	if err != nil {
		return "", err
	}
	return obj.Id, nil
}

func routeID(kong *pdk.PDK) (string, error) {
	obj, err := kong.Router.GetRoute()
	if err != nil {
		return "", err
	}
	return obj.Id, nil
}

func NewCacheKey(kong *pdk.PDK, conf *Config, body []byte, cacheTTL int) (string, error) {
	logger := conf.logger

	consumerID, err := consumerID(kong)
	if err != nil {
		logger.Error().Err(err).Msg("Getting consumerID has failed")
	}
	if consumerID == "" {
		logger.Debug().Msg("consumerID is empty")
	}

	serviceID, err := serviceID(kong)
	if err != nil {
		logger.Error().Err(err).Msg("Getting serviceID has failed")
	}
	if serviceID == "" {
		logger.Debug().Msg("serviceID is empty")
	}

	routeID, err := routeID(kong)
	if err != nil {
		logger.Error().Err(err).Msg("Getting routeID has failed")
	}
	if routeID == "" {
		logger.Debug().Msg("routeID is empty")
	}

	method, err := kong.Request.GetMethod()
	if err != nil {
		logger.Error().Err(err).Msg("Getting method has failed")
	}
	if method == "" {
		logger.Debug().Msg("method is empty")
	}

	path, err := kong.Request.GetPath()
	if err != nil {
		logger.Error().Err(err).Msg("Getting path has failed")
	}
	if path == "" {
		logger.Debug().Msg("path is empty")
	}

	//pathWithQuery, err := kong.Request.GetPathWithQuery()
	//if err != nil {
	//	_ = log.Err("pathWithQuery is not set")
	//}
	//_ = log.Debug("pathWithQuery is: ", pathWithQuery)

	headers := make(map[string]string)
	for _, v := range conf.VaryHeaders {
		headerValue, error := kong.Request.GetHeader(v)
		if headerValue == "" || error != nil {
			logger.Debug().Msgf("vary header %s is not found", v)
			continue
		}
		headers[v] = headerValue
		logger.Debug().Msgf("vary header %s is found: %s", v, headerValue)
	}

	queryArgs, err := kong.Request.GetQuery(1000)
	if err != nil {
		logger.Error().Err(err).Msg("Getting queryArgs has failed")
		//return
	}
	if conf.isDebug() {
		if queryArgs == nil {
			logger.Debug().Msg("queryArgs is empty")
		} else {
			for key, v := range queryArgs {
				logger.Debug().Msgf("Query arg %s is found: %s", key, v)
			}
		}
	}

	cacheKey := &CacheKey{
		Consumer:  consumerID,
		Service:   serviceID,
		Route:     routeID,
		Method:    method,
		URL:       path,
		Headers:   headers,
		QueryArgs: queryArgs,
		Body:      body,
		CacheTTL:  cacheTTL,
	}

	validate := validator.New()
	if errs := validate.Struct(cacheKey); errs != nil {
		logger.Error().Err(errs).Msg("validation error")
		return "", errs
	}

	return generateCacheKeyID(logger, cacheKey)
}

func generateCacheKeyID(logger *Logger, cacheKey *CacheKey) (string, error) {
	hash, err := hashstructure.Hash(cacheKey, hashstructure.FormatV2, nil)
	if err != nil {
		logger.Error().Err(err).Msg("hashing error")
		return "", err
	}

	cacheKeyID := strconv.FormatUint(hash, 10)
	logger.Debug().Msgf("cache key id %s is generated from: %v", cacheKeyID, cacheKey)
	return cacheKeyID, nil
}

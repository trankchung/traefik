package provider

import (
	"bytes"
	"context"
	"reflect"
	"sort"
	"strings"
	"text/template"
	"unicode"

	"github.com/Masterminds/sprig"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/containous/traefik/v2/pkg/log"
)

// Merge Merges multiple configurations.
func Merge(ctx context.Context, configurations map[string]*dynamic.Configuration) *dynamic.Configuration {
	logger := log.FromContext(ctx)

	configuration := &dynamic.Configuration{
		HTTP: &dynamic.HTTPConfiguration{
			Routers:     make(map[string]*dynamic.Router),
			Middlewares: make(map[string]*dynamic.Middleware),
			Services:    make(map[string]*dynamic.Service),
		},
		TCP: &dynamic.TCPConfiguration{
			Routers:  make(map[string]*dynamic.TCPRouter),
			Services: make(map[string]*dynamic.TCPService),
		},
		UDP: &dynamic.UDPConfiguration{
			Routers:  make(map[string]*dynamic.UDPRouter),
			Services: make(map[string]*dynamic.UDPService),
		},
	}

	servicesToDelete := map[string]struct{}{}
	services := map[string][]string{}

	routersToDelete := map[string]struct{}{}
	routers := map[string][]string{}

	servicesTCPToDelete := map[string]struct{}{}
	servicesTCP := map[string][]string{}

	routersTCPToDelete := map[string]struct{}{}
	routersTCP := map[string][]string{}

	middlewaresToDelete := map[string]struct{}{}
	middlewares := map[string][]string{}

	var sortedKeys []string
	for key := range configurations {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	for _, root := range sortedKeys {
		conf := configurations[root]
		for serviceName, service := range conf.HTTP.Services {
			services[serviceName] = append(services[serviceName], root)
			if !AddService(configuration.HTTP, serviceName, service) {
				servicesToDelete[serviceName] = struct{}{}
			}
		}

		for routerName, router := range conf.HTTP.Routers {
			routers[routerName] = append(routers[routerName], root)
			if !AddRouter(configuration.HTTP, routerName, router) {
				routersToDelete[routerName] = struct{}{}
			}
		}

		for serviceName, service := range conf.TCP.Services {
			servicesTCP[serviceName] = append(servicesTCP[serviceName], root)
			if !AddServiceTCP(configuration.TCP, serviceName, service) {
				servicesTCPToDelete[serviceName] = struct{}{}
			}
		}

		for routerName, router := range conf.TCP.Routers {
			routersTCP[routerName] = append(routersTCP[routerName], root)
			if !AddRouterTCP(configuration.TCP, routerName, router) {
				routersTCPToDelete[routerName] = struct{}{}
			}
		}

		for middlewareName, middleware := range conf.HTTP.Middlewares {
			middlewares[middlewareName] = append(middlewares[middlewareName], root)
			if !AddMiddleware(configuration.HTTP, middlewareName, middleware) {
				middlewaresToDelete[middlewareName] = struct{}{}
			}
		}
	}

	for serviceName := range servicesToDelete {
		logger.WithField(log.ServiceName, serviceName).
			Errorf("Service defined multiple times with different configurations in %v", services[serviceName])
		delete(configuration.HTTP.Services, serviceName)
	}

	for routerName := range routersToDelete {
		logger.WithField(log.RouterName, routerName).
			Errorf("Router defined multiple times with different configurations in %v", routers[routerName])
		delete(configuration.HTTP.Routers, routerName)
	}

	for serviceName := range servicesTCPToDelete {
		logger.WithField(log.ServiceName, serviceName).
			Errorf("Service TCP defined multiple times with different configurations in %v", servicesTCP[serviceName])
		delete(configuration.TCP.Services, serviceName)
	}

	for routerName := range routersTCPToDelete {
		logger.WithField(log.RouterName, routerName).
			Errorf("Router TCP defined multiple times with different configurations in %v", routersTCP[routerName])
		delete(configuration.TCP.Routers, routerName)
	}

	for middlewareName := range middlewaresToDelete {
		logger.WithField(log.MiddlewareName, middlewareName).
			Errorf("Middleware defined multiple times with different configurations in %v", middlewares[middlewareName])
		delete(configuration.HTTP.Middlewares, middlewareName)
	}

	return configuration
}

// AddServiceTCP Adds a service to a configurations.
func AddServiceTCP(configuration *dynamic.TCPConfiguration, serviceName string, service *dynamic.TCPService) bool {
	if _, ok := configuration.Services[serviceName]; !ok {
		configuration.Services[serviceName] = service
		return true
	}

	if !configuration.Services[serviceName].LoadBalancer.Mergeable(service.LoadBalancer) {
		return false
	}

	configuration.Services[serviceName].LoadBalancer.Servers = append(configuration.Services[serviceName].LoadBalancer.Servers, service.LoadBalancer.Servers...)
	return true
}

// AddRouterTCP Adds a router to a configurations.
func AddRouterTCP(configuration *dynamic.TCPConfiguration, routerName string, router *dynamic.TCPRouter) bool {
	if _, ok := configuration.Routers[routerName]; !ok {
		configuration.Routers[routerName] = router
		return true
	}

	return reflect.DeepEqual(configuration.Routers[routerName], router)
}

// AddService Adds a service to a configurations.
func AddService(configuration *dynamic.HTTPConfiguration, serviceName string, service *dynamic.Service) bool {
	if _, ok := configuration.Services[serviceName]; !ok {
		configuration.Services[serviceName] = service
		return true
	}

	if !configuration.Services[serviceName].LoadBalancer.Mergeable(service.LoadBalancer) {
		return false
	}

	configuration.Services[serviceName].LoadBalancer.Servers = append(configuration.Services[serviceName].LoadBalancer.Servers, service.LoadBalancer.Servers...)
	return true
}

// AddRouter Adds a router to a configurations.
func AddRouter(configuration *dynamic.HTTPConfiguration, routerName string, router *dynamic.Router) bool {
	if _, ok := configuration.Routers[routerName]; !ok {
		configuration.Routers[routerName] = router
		return true
	}

	return reflect.DeepEqual(configuration.Routers[routerName], router)
}

// AddMiddleware Adds a middleware to a configurations.
func AddMiddleware(configuration *dynamic.HTTPConfiguration, middlewareName string, middleware *dynamic.Middleware) bool {
	if _, ok := configuration.Middlewares[middlewareName]; !ok {
		configuration.Middlewares[middlewareName] = middleware
		return true
	}

	return reflect.DeepEqual(configuration.Middlewares[middlewareName], middleware)
}

// MakeDefaultRuleTemplate Creates the default rule template.
func MakeDefaultRuleTemplate(defaultRule string, funcMap template.FuncMap) (*template.Template, error) {
	defaultFuncMap := sprig.TxtFuncMap()
	defaultFuncMap["normalize"] = Normalize

	for k, fn := range funcMap {
		defaultFuncMap[k] = fn
	}

	return template.New("defaultRule").Funcs(defaultFuncMap).Parse(defaultRule)
}

// BuildTCPRouterConfiguration Builds a router configuration.
func BuildTCPRouterConfiguration(ctx context.Context, configuration *dynamic.TCPConfiguration) {
	for routerName, router := range configuration.Routers {
		loggerRouter := log.FromContext(ctx).WithField(log.RouterName, routerName)
		if len(router.Rule) == 0 {
			delete(configuration.Routers, routerName)
			loggerRouter.Errorf("Empty rule")
			continue
		}

		if len(router.Service) == 0 {
			if len(configuration.Services) > 1 {
				delete(configuration.Routers, routerName)
				loggerRouter.
					Error("Could not define the service name for the router: too many services")
				continue
			}

			for serviceName := range configuration.Services {
				router.Service = serviceName
			}
		}
	}
}

// BuildRouterConfiguration Builds a router configuration.
func BuildRouterConfiguration(ctx context.Context, configuration *dynamic.HTTPConfiguration, defaultRouterName string, defaultRuleTpl *template.Template, model interface{}) {
	if len(configuration.Routers) == 0 {
		if len(configuration.Services) > 1 {
			log.FromContext(ctx).Info("Could not create a router for the container: too many services")
		} else {
			configuration.Routers = make(map[string]*dynamic.Router)
			configuration.Routers[defaultRouterName] = &dynamic.Router{}
		}
	}

	for routerName, router := range configuration.Routers {
		loggerRouter := log.FromContext(ctx).WithField(log.RouterName, routerName)
		if len(router.Rule) == 0 {
			writer := &bytes.Buffer{}
			if err := defaultRuleTpl.Execute(writer, model); err != nil {
				loggerRouter.Errorf("Error while parsing default rule: %v", err)
				delete(configuration.Routers, routerName)
				continue
			}

			router.Rule = writer.String()
			if len(router.Rule) == 0 {
				loggerRouter.Error("Undefined rule")
				delete(configuration.Routers, routerName)
				continue
			}
		}

		if len(router.Service) == 0 {
			if len(configuration.Services) > 1 {
				delete(configuration.Routers, routerName)
				loggerRouter.
					Error("Could not define the service name for the router: too many services")
				continue
			}

			for serviceName := range configuration.Services {
				router.Service = serviceName
			}
		}
	}
}

// Normalize Replace all special chars with `-`.
func Normalize(name string) string {
	fargs := func(c rune) bool {
		return !unicode.IsLetter(c) && !unicode.IsNumber(c)
	}
	// get function
	return strings.Join(strings.FieldsFunc(name, fargs), "-")
}

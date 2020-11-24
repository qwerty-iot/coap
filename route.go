package coap

import (
	"strings"
)

type RouteCallback func(req *Message) *Message

var routes = make(map[string]*routeEntry)

type routeEntry struct {
	children map[string]*routeEntry
	key      string
	callback RouteCallback
}

func AddRoute(path string, callback RouteCallback) {
	logInfo(nil, "Add route for: %s", path)
	pathParts := strings.Split(path, "/")
	var route *routeEntry
	var found bool
	routeMap := routes
	for idx, part := range pathParts {
		if len(part) == 0 {
			continue
		}
		var key string
		if part[0] == '{' {
			key = part[1 : len(part)-1]
			part = "*"
		}

		if route, found = routeMap[part]; found {
			if idx == len(pathParts)-1 {
				route.callback = callback
			} else {
				routeMap = route.children
			}
		} else {
			if idx == len(pathParts)-1 {
				route = &routeEntry{children: map[string]*routeEntry{}, callback: callback, key: key}
			} else {
				route = &routeEntry{children: map[string]*routeEntry{}, key: key}
			}
			routeMap[part] = route
		}
	}
}

func matchRoutes(msg *Message) RouteCallback {

	pathParts := strings.Split(msg.PathString(), "/")

	var route *routeEntry
	var found bool
	routeMap := routes
	var deepestCallback RouteCallback
	for _, part := range pathParts {
		if route, found = routeMap[part]; found {
			deepestCallback = route.callback
			routeMap = route.children
		} else {
			if route, found = routeMap["*"]; found {
				deepestCallback = route.callback
				if msg.PathVars == nil {
					msg.PathVars = map[string]string{}
				}
				msg.PathVars[route.key] = part
			} else {
				break
			}
		}
	}
	return deepestCallback
}

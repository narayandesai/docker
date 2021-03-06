package api

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/engine"
	"github.com/docker/docker/pkg/parsers"
	"github.com/docker/docker/pkg/version"
	"github.com/docker/libtrust"
)

// Common constants for daemon and client.
const (
	APIVERSION            version.Version = "1.19"                 // Current REST API version
	DEFAULTHTTPHOST                       = "127.0.0.1"            // Default HTTP Host used if only port is provided to -H flag e.g. docker -d -H tcp://:8080
	DEFAULTUNIXSOCKET                     = "/var/run/docker.sock" // Docker daemon by default always listens on the default unix socket
	DefaultDockerfileName string          = "Dockerfile"           // Default filename with Docker commands, read by docker build
)

func ValidateHost(val string) (string, error) {
	host, err := parsers.ParseHost(DEFAULTHTTPHOST, DEFAULTUNIXSOCKET, val)
	if err != nil {
		return val, err
	}
	return host, nil
}

// TODO remove, used on < 1.5 in getContainersJSON
// TODO this can go away when we get rid of engine.table
func DisplayablePorts(ports *engine.Table) string {
	var (
		result          = []string{}
		hostMappings    = []string{}
		firstInGroupMap map[string]int
		lastInGroupMap  map[string]int
	)
	firstInGroupMap = make(map[string]int)
	lastInGroupMap = make(map[string]int)
	ports.SetKey("PrivatePort")
	ports.Sort()
	for _, port := range ports.Data {
		var (
			current      = port.GetInt("PrivatePort")
			portKey      = port.Get("Type")
			firstInGroup int
			lastInGroup  int
		)
		if port.Get("IP") != "" {
			if port.GetInt("PublicPort") != current {
				hostMappings = append(hostMappings, fmt.Sprintf("%s:%d->%d/%s", port.Get("IP"), port.GetInt("PublicPort"), port.GetInt("PrivatePort"), port.Get("Type")))
				continue
			}
			portKey = fmt.Sprintf("%s/%s", port.Get("IP"), port.Get("Type"))
		}
		firstInGroup = firstInGroupMap[portKey]
		lastInGroup = lastInGroupMap[portKey]

		if firstInGroup == 0 {
			firstInGroupMap[portKey] = current
			lastInGroupMap[portKey] = current
			continue
		}

		if current == (lastInGroup + 1) {
			lastInGroupMap[portKey] = current
			continue
		}
		result = append(result, FormGroup(portKey, firstInGroup, lastInGroup))
		firstInGroupMap[portKey] = current
		lastInGroupMap[portKey] = current
	}
	for portKey, firstInGroup := range firstInGroupMap {
		result = append(result, FormGroup(portKey, firstInGroup, lastInGroupMap[portKey]))
	}
	result = append(result, hostMappings...)
	return strings.Join(result, ", ")
}

type ByPrivatePort []types.Port

func (r ByPrivatePort) Len() int           { return len(r) }
func (r ByPrivatePort) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r ByPrivatePort) Less(i, j int) bool { return r[i].PrivatePort < r[j].PrivatePort }

// TODO Rename to DisplayablePorts (remove "New") when engine.Table goes away
func NewDisplayablePorts(ports []types.Port) string {
	var (
		result          = []string{}
		hostMappings    = []string{}
		firstInGroupMap map[string]int
		lastInGroupMap  map[string]int
	)
	firstInGroupMap = make(map[string]int)
	lastInGroupMap = make(map[string]int)
	sort.Sort(ByPrivatePort(ports))
	for _, port := range ports {
		var (
			current      = port.PrivatePort
			portKey      = port.Type
			firstInGroup int
			lastInGroup  int
		)
		if port.IP != "" {
			if port.PublicPort != current {
				hostMappings = append(hostMappings, fmt.Sprintf("%s:%d->%d/%s", port.IP, port.PublicPort, port.PrivatePort, port.Type))
				continue
			}
			portKey = fmt.Sprintf("%s/%s", port.IP, port.Type)
		}
		firstInGroup = firstInGroupMap[portKey]
		lastInGroup = lastInGroupMap[portKey]

		if firstInGroup == 0 {
			firstInGroupMap[portKey] = current
			lastInGroupMap[portKey] = current
			continue
		}

		if current == (lastInGroup + 1) {
			lastInGroupMap[portKey] = current
			continue
		}
		result = append(result, FormGroup(portKey, firstInGroup, lastInGroup))
		firstInGroupMap[portKey] = current
		lastInGroupMap[portKey] = current
	}
	for portKey, firstInGroup := range firstInGroupMap {
		result = append(result, FormGroup(portKey, firstInGroup, lastInGroupMap[portKey]))
	}
	result = append(result, hostMappings...)
	return strings.Join(result, ", ")
}

func FormGroup(key string, start, last int) string {
	var (
		group     string
		parts     = strings.Split(key, "/")
		groupType = parts[0]
		ip        = ""
	)
	if len(parts) > 1 {
		ip = parts[0]
		groupType = parts[1]
	}
	if start == last {
		group = fmt.Sprintf("%d", start)
	} else {
		group = fmt.Sprintf("%d-%d", start, last)
	}
	if ip != "" {
		group = fmt.Sprintf("%s:%s->%s", ip, group, group)
	}
	return fmt.Sprintf("%s/%s", group, groupType)
}

func MatchesContentType(contentType, expectedType string) bool {
	mimetype, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		logrus.Errorf("Error parsing media type: %s error: %v", contentType, err)
	}
	return err == nil && mimetype == expectedType
}

// LoadOrCreateTrustKey attempts to load the libtrust key at the given path,
// otherwise generates a new one
func LoadOrCreateTrustKey(trustKeyPath string) (libtrust.PrivateKey, error) {
	err := os.MkdirAll(filepath.Dir(trustKeyPath), 0700)
	if err != nil {
		return nil, err
	}
	trustKey, err := libtrust.LoadKeyFile(trustKeyPath)
	if err == libtrust.ErrKeyFileDoesNotExist {
		trustKey, err = libtrust.GenerateECP256PrivateKey()
		if err != nil {
			return nil, fmt.Errorf("Error generating key: %s", err)
		}
		if err := libtrust.SaveKey(trustKeyPath, trustKey); err != nil {
			return nil, fmt.Errorf("Error saving key file: %s", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("Error loading key file %s: %s", trustKeyPath, err)
	}
	return trustKey, nil
}

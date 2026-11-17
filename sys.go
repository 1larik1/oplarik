package main

// Sys abstracts every system call the engine needs, so the same engine logic
// runs against real Windows (sys_windows.go) or an in-memory simulation used
// for demo mode and tests (sys_mock.go).
type Sys interface {
	RegGet(key, name string) (RegVal, error)
	RegSet(key, name string, v RegVal) error
	RegDelete(key, name string) error
	RegKeyExists(key string) bool
	RegCreateKey(key string) error
	RegDeleteTree(key string) error
	RegSubkeys(key string) ([]string, error)

	SvcStart(name string) (start string, exists bool)
	SvcConfig(name, start string) error
	SvcStop(name string)
	SvcRun(name string)

	TaskState(path string) (exists, enabled bool)
	TaskSet(path string, enabled bool) error

	PowerGet(sub, set string) (ac, dc int, ok bool)
	PowerSet(sub, set string, ac, dc int, setDC bool) error
	ActivePlan() string
	PlanExists(guid string) bool

	Exec(args ...string) (string, error)
	PSRun(id, mode, script, backup string) (string, error)
	PSChecks(checks map[string]string) map[string]string

	AppxInstalled() ([]string, error)
	AppxRemove(patterns []string) error
	Features(names []string) (map[string]string, error)
	FeatureSet(name string, enable bool) error
	Caps(prefixes []string) (map[string]CapInfo, error)
	CapRemove(name string) error
	CapAdd(name string) error

	OpenURL(url string)
}

type CapInfo struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

const (
	planOurs     = "4c61726b-5052-4f00-8000-000000000001"
	planUltimate = "e9a42b02-d5df-448d-aa00-03f14749eb61"
	planHighPerf = "8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c"
	planBalanced = "381b4222-f694-41f0-9685-ff5bb260df2e"
)

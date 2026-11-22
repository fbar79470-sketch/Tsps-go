package main

import (
	"fmt"

	"tsps-go/internal/cache"
	"tsps-go/internal/content/doors"
)

// installDoorsCatalog builds the door catalog from the loaded cache
// definitions and installs it on the Doors content package, before hub
// Load (see newPluginHub): Register reads the installed catalog when
// it builds the toggle engine, so a call after Load is too late. A
// catalog build failure fails the boot, matching loadDefinitions: a
// cache whose gameval names do not resolve is a config error, not a
// degradation. With nil defs (NoCache mode) the catalog stays
// uninstalled and every door toggle falls through — doors stay shut —
// the same no-definitions degradation as the rest of the server.
func installDoorsCatalog(defs *cache.CacheDefinitions) error {
	if defs == nil {
		return nil
	}
	catalog, err := doors.BuildCatalog(cacheCatalogDefs{defs: defs})
	if err != nil {
		return fmt.Errorf("loading door catalog: %w", err)
	}
	doors.SetCatalog(catalog)
	return nil
}

// cacheCatalogDefs adapts *cache.CacheDefinitions to the
// doors.CatalogDefs surface BuildCatalog scans through: GetObject and
// ObjectCount straight off the facade, LocName over the gameval loc
// names ("" when the cache names none), LocID over the gameval loc
// ids. A nil defs reads as an empty cache.
type cacheCatalogDefs struct {
	defs *cache.CacheDefinitions
}

func (a cacheCatalogDefs) GetObject(id int) *cache.LocType {
	if a.defs == nil {
		return nil
	}
	return a.defs.GetObject(id)
}

func (a cacheCatalogDefs) ObjectCount() int {
	if a.defs == nil {
		return 0
	}
	return a.defs.GetBaseCounts().Objects
}

func (a cacheCatalogDefs) LocName(id int) string {
	if a.defs == nil {
		return ""
	}
	names, err := a.defs.GetLocNames()
	if err != nil {
		return ""
	}
	return names[id]
}

func (a cacheCatalogDefs) LocID(name string) (int, bool) {
	if a.defs == nil {
		return 0, false
	}
	id, ok, err := a.defs.GetLocId(name)
	if err != nil {
		return 0, false
	}
	return id, ok
}

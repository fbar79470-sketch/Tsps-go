package definition

import "fmt"

// Interface layout opening: the port of
// InterfaceLayoutRegistry.open (InterfaceLayoutDefinition.ts:
// 75-127) plus the require/get lookups it builds on
// (:51-72). The loader and the Store queries live in
// layouts.go; this file adds only the open dispatch, which
// needs a packet sender and therefore could not live with the
// data loading. The sender is the narrow InterfacePacketSender
// interface, satisfied by *session.Session (which cannot be
// imported here without a cycle).

// InterfacePacketSender is the sender surface a layout open
// needs (InterfacePacketSender, InterfaceLayoutDefinition.ts:
// 20-27), in Go signatures: SendChatboxInterface takes the
// unclamp flag the Go sender requires, and open always passes
// false (TS's single-argument call mounts clamped).
type InterfacePacketSender interface {
	SendInterface(interfaceID int)
	SendChatboxInterface(interfaceID int, unclamp bool)
	SendWalkableInterface(interfaceID int)
	SendTabInterface(tabID, interfaceID int)
	SendSidebarInterface(interfaceID int)
	SendInterfaceSet(interfaceID, sidebarInterfaceID int)
}

// LayoutLookup is the layout resolution a layout open needs:
// the Store's key and id queries (InterfaceLayoutRegistry.get,
// InterfaceLayoutDefinition.ts:51-58). *Store satisfies it;
// tests use hand-built fakes.
type LayoutLookup interface {
	Layout(key string) (*InterfaceLayoutDefinition, bool)
	LayoutByID(id int) (*InterfaceLayoutDefinition, bool)
}

// GetLayout resolves a reference to a layout
// (InterfaceLayoutRegistry.get): a string key resolves by key,
// an int id resolves to the first definition sharing the id.
// Any other reference type resolves to nothing: TS's reference
// is string | number, and nothing else can arrive there.
func GetLayout(lookup LayoutLookup, reference any) (*InterfaceLayoutDefinition, bool) {
	if lookup == nil {
		return nil, false
	}
	switch ref := reference.(type) {
	case string:
		return lookup.Layout(ref)
	case int:
		return lookup.LayoutByID(ref)
	default:
		return nil, false
	}
}

// RequireLayout resolves a reference or returns the
// unknown-layout error (InterfaceLayoutRegistry.require,
// InterfaceLayoutDefinition.ts:60-67).
func RequireLayout(lookup LayoutLookup, reference any) (*InterfaceLayoutDefinition, error) {
	def, ok := GetLayout(lookup, reference)
	if !ok {
		return nil, fmt.Errorf("Unknown interface layout: %v", reference)
	}
	return def, nil
}

// OpenLayout opens a layout on its surface and returns it
// (InterfaceLayoutRegistry.open,
// InterfaceLayoutDefinition.ts:75-127):
//   - MAIN / FULLSCREEN: sendInterface(id).
//   - CHATBOX: sendChatboxInterface(id), clamped.
//   - WALKABLE_OVERLAY: sendWalkableInterface(id).
//   - SIDEBAR: sendTabInterface(tabId, id); a missing,
//     non-integer, or negative tabId is an error.
//   - SIDEBAR_OVERLAY: sendSidebarInterface(id).
//   - MAIN_WITH_SIDEBAR: sendInterfaceSet(id,
//     sidebarInterfaceId); a missing, non-integer, or negative
//     sidebarInterfaceId is an error.
//
// An unknown reference, a surface-specific id failure, or an
// unknown surface returns an error with the TS message and
// sends nothing. The loader (layouts.go) already validates
// tabId/sidebarInterfaceId for the surfaces that need them, so
// the open-time checks fire only for hand-built definitions —
// but TS validates at open time too, and this ports that
// check, including the exact messages. TabID /
// SidebarInterfaceID are *int in Go, so "non-integer" can only
// mean "absent" here: the loader drops non-numeric values (see
// validateLayout), which open then rejects as missing.
func OpenLayout(lookup LayoutLookup, sender InterfacePacketSender, reference any) (*InterfaceLayoutDefinition, error) {
	def, err := RequireLayout(lookup, reference)
	if err != nil {
		return nil, err
	}
	switch def.Surface {
	case SurfaceMain, SurfaceFullscreen:
		sender.SendInterface(def.ID)
	case SurfaceChatbox:
		sender.SendChatboxInterface(def.ID, false)
	case SurfaceWalkableOverlay:
		sender.SendWalkableInterface(def.ID)
	case SurfaceSidebar:
		if def.TabID == nil || *def.TabID < 0 {
			return nil, fmt.Errorf("Sidebar interface %s requires a tabId", def.Key)
		}
		sender.SendTabInterface(*def.TabID, def.ID)
	case SurfaceSidebarOverlay:
		sender.SendSidebarInterface(def.ID)
	case SurfaceMainWithSidebar:
		if def.SidebarInterfaceID == nil || *def.SidebarInterfaceID < 0 {
			return nil, fmt.Errorf("Interface %s requires a sidebarInterfaceId", def.Key)
		}
		sender.SendInterfaceSet(def.ID, *def.SidebarInterfaceID)
	default:
		return nil, fmt.Errorf("Unsupported interface surface: %s", def.Surface)
	}
	return def, nil
}

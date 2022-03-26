package mgba

/*
#include <mgba/core/core.h>

size_t bbn6_mgba_mCore_stateSize(struct mCore* core) {
	return core->stateSize(core);
}

bool bbn6_mgba_mCore_saveState(struct mCore* core, void* state) {
	return core->saveState(core, state);
}

bool bbn6_mgba_mCore_loadState(struct mCore* core, void* state) {
	return core->loadState(core, state);
}
*/
import "C"
import (
	"runtime"
	"unsafe"
)

type State struct {
	ptr  unsafe.Pointer
	size int
}

func (c *Core) SaveState() *State {
	size := int(C.bbn6_mgba_mCore_stateSize(c.ptr))
	buf := unsafe.Pointer(C.malloc(C.size_t(size)))
	ok := C.bbn6_mgba_mCore_saveState(c.ptr, buf)
	if !ok {
		C.free(buf)
		return nil
	}

	s := &State{buf, size}
	runtime.SetFinalizer(s, func(s *State) {
		C.free(s.ptr)
	})
	return s
}

func (c *Core) LoadState(state *State) bool {
	return bool(C.bbn6_mgba_mCore_loadState(c.ptr, state.ptr))
}

func (s *State) Bytes() []byte {
	return C.GoBytes(s.ptr, C.int(s.size))
}

func StateFromBytes(b []byte) *State {
	s := &State{C.CBytes(b), len(b)}
	runtime.SetFinalizer(s, func(s *State) {
		C.free(s.ptr)
	})
	return s
}

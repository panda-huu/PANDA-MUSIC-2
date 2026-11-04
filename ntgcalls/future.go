package ntgcalls

/*
#include "ntgcalls.h"
#include <stdlib.h>
extern void unlockMutex(void*);
*/
import "C"

import (
	"runtime/cgo"
	"sync"
	"unsafe"
)

type Future struct {
	mutex      *sync.Mutex
	errCode    *C.int
	errMessage **C.char
	handle     cgo.Handle
	userData   unsafe.Pointer
}

func CreateFuture() *Future {
	res := &Future{
		mutex:      &sync.Mutex{},
		errCode:    (*C.int)(C.calloc(1, C.size_t(unsafe.Sizeof(C.int(0))))),
		errMessage: (**C.char)(C.calloc(1, C.size_t(unsafe.Sizeof((*C.char)(nil))))),
	}
	res.mutex.Lock()
	return res
}

func (ctx *Future) ParseToC() C.ntg_async_struct {
	var x C.ntg_async_struct
	if ctx.userData == nil {
		ctx.handle = cgo.NewHandle(ctx)
		ctx.userData = C.malloc(C.size_t(unsafe.Sizeof(C.uintptr_t(0))))
		*(*C.uintptr_t)(ctx.userData) = C.uintptr_t(ctx.handle)
	}
	x.userData = ctx.userData
	x.promise = (C.ntg_async_callback)(unsafe.Pointer(C.unlockMutex))
	x.errorCode = ctx.errCode
	x.errorMessage = ctx.errMessage
	return x
}

func (ctx *Future) wait() {
	ctx.mutex.Lock()
}

//export unlockMutex
func unlockMutex(p unsafe.Pointer) {
	if p == nil {
		return
	}
	h := cgo.Handle(*(*C.uintptr_t)(p))
	future, _ := h.Value().(*Future)
	if future != nil && future.mutex != nil {
		future.mutex.Unlock()
	}
	h.Delete()
	C.free(p)
}

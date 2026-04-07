package main

/*
#cgo pkg-config: sdl3-ttf
#include <SDL3_ttf/SDL_ttf.h>
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"unsafe"
)

func TTF3Init() error {
	if !C.TTF_Init() {
		return fmt.Errorf("TTF_Init: %s", C.GoString(C.SDL_GetError()))
	}
	return nil
}

func TTF3OpenFont(path string, ptsize float32) (*Font, error) {
	cp := C.CString(path)
	defer C.free(unsafe.Pointer(cp))
	f := C.TTF_OpenFont(cp, C.float(ptsize))
	if f == nil {
		return nil, fmt.Errorf("TTF_OpenFont(%s): %s", path, C.GoString(C.SDL_GetError()))
	}
	return &Font{ptr: unsafe.Pointer(f)}, nil
}

func TTF3CloseFont(f *Font) {
	if f != nil && f.ptr != nil {
		C.TTF_CloseFont((*C.TTF_Font)(f.ptr))
	}
}

// TTF3RenderTextBlended renders text to a new surface with blended (anti-aliased) quality.
func TTF3RenderTextBlended(f *Font, text string, color Color) (*Surface, error) {
	ct := C.CString(text)
	defer C.free(unsafe.Pointer(ct))
	cc := C.SDL_Color{r: C.Uint8(color.R), g: C.Uint8(color.G), b: C.Uint8(color.B), a: C.Uint8(color.A)}
	surf := C.TTF_RenderText_Blended((*C.TTF_Font)(f.ptr), ct, 0, cc)
	if surf == nil {
		return nil, fmt.Errorf("TTF_RenderText_Blended: %s", C.GoString(C.SDL_GetError()))
	}
	return &Surface{ptr: unsafe.Pointer(surf)}, nil
}

// TTF3GetStringSize returns the width and height of the rendered text.
func TTF3GetStringSize(f *Font, text string) (int32, int32) {
	ct := C.CString(text)
	defer C.free(unsafe.Pointer(ct))
	var w, h C.int
	C.TTF_GetStringSize((*C.TTF_Font)(f.ptr), ct, 0, &w, &h)
	return int32(w), int32(h)
}

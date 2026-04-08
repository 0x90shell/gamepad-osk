package main

import "unsafe"

// Color matches SDL_Color layout: 4 contiguous uint8s.
type Color struct{ R, G, B, A uint8 }

// Rect matches SDL_Rect layout: 4 contiguous int32s.
type Rect struct{ X, Y, W, H int32 }

// FRect matches SDL_FRect layout: 4 contiguous float32s.
// SDL3 render functions use FRect instead of Rect.
type FRect struct{ X, Y, W, H float32 }

// Opaque handle types wrapping C pointers from SDL3.
// These are never dereferenced in Go - only passed back to cgo wrappers.

// Window is an opaque handle to an SDL_Window.
type Window struct{ ptr unsafe.Pointer }

// Renderer is an opaque handle to an SDL_Renderer.
type SDLRenderer struct{ ptr unsafe.Pointer }

// Texture is an opaque handle to an SDL_Texture.
type Texture struct{ ptr unsafe.Pointer }

// Surface is an opaque handle to an SDL_Surface.
type Surface struct{ ptr unsafe.Pointer }

// Font is an opaque handle to a TTF_Font.
type Font struct{ ptr unsafe.Pointer }

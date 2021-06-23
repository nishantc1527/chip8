package main

import (
	"github.com/go-gl/glfw/v3.3/glfw"
	"runtime"
)

type chip8 struct {
	memory[0x1000]int8

	v[0xf]int8
	i int16
	dt int8
	st int8
	pc int16
	sp int8

	stack[0xf]int16

	instruction int16
}

func init() {
	// This is needed to arrange that main() runs on main thread.
	// See documentation for functions that are only allowed to be called from the main thread.
	runtime.LockOSThread()
}

func main() {
	err := glfw.Init()

	if err != nil {
		panic(err)
	}

	defer glfw.Terminate()

	window, err := glfw.CreateWindow(64, 32, "Testing", nil, nil)

	if err != nil {
		panic(err)
	}

	window.MakeContextCurrent()

	for !window.ShouldClose() {
		window.SwapBuffers()
		glfw.PollEvents()
	}
}

//go:build darwin

package main

/*
*/
import "C"
import "github.com/wailsapp/wails/v2/pkg/runtime"

//export goTrayOnClick
func goTrayOnClick(tag C.int) {
	handleTrayClick(int(tag))
}

//export goSetForceQuit
func goSetForceQuit() {
	forceQuit.Store(true)
}

//export goTrayReopen
func goTrayReopen() {
	runtime.WindowShow(mainAppCtx)
	runtime.WindowUnminimise(mainAppCtx)
}

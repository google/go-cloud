// Code generated by Wire. DO NOT EDIT.

//go:generate wire
//+build !wireinject

package main

import (
	"example.com/foo"
)

// Injectors from wire.go:

func injectFooer() foo.Fooer {
	bar := provideBar()
	return bar
}

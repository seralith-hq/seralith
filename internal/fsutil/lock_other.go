//go:build !unix

package fsutil

import "os"

func Lock(*os.File) error   { return nil }
func Unlock(*os.File) error { return nil }

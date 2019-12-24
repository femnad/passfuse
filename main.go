package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/alexflint/go-arg"
	"github.com/femnad/passfuse/pkg/fs"
	"github.com/jacobsa/fuse"
	"os"
	"os/signal"
)

var args struct {
	MountPath       string `default:"/tmp/.passfuse" arg:"-m"`
	CreateMountPath bool   `default:"true" arg:"-c"`
}

func main() {
	arg.MustParse(&args)
	server, _ := fs.NewPassFS()
	cfg := &fuse.MountConfig{}
	_, err := os.Stat(args.MountPath)
	if errors.Is(err, os.ErrNotExist) && args.CreateMountPath {
		err = os.MkdirAll(args.MountPath, 0755)
		if err != nil {
			panic(err)
		}
	}
	mountedFS, err := fuse.Mount(args.MountPath, server, cfg)
	if err != nil {
		panic(err)
	}
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		for {
			<-sigChan
			err := fuse.Unmount(args.MountPath)
			if err != nil {
				fmt.Printf("Unmount error %v\n", err)
			} else {
				break
			}
		}
	}()
	err = mountedFS.Join(context.Background())
	if err != nil {
		panic(err)
	}
}

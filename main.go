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
	"time"
)

const (
	mountPathPermission = 0700
)

var args struct {
	CreateMountPath bool   `default:"true" arg:"-c"`
	MountPath       string `default:"$HOME/.mnt/passfuse" arg:"-m"`
	PasswordStorePath string `arg:"-p"`
	UnmountAfter int `arg:"-u"`
}

func main() {
	arg.MustParse(&args)

	server, err := fs.NewPassFS(args.PasswordStorePath)
	if err != nil {
		fmt.Printf("Error initializing filesystem %s\n", err)
		os.Exit(1)
	}

	cfg := &fuse.MountConfig{}
	mountPath := os.ExpandEnv(args.MountPath)
	_, err = os.Stat(mountPath)
	if errors.Is(err, os.ErrNotExist) && args.CreateMountPath {
		err = os.MkdirAll(mountPath, mountPathPermission)
		if err != nil {
			panic(err)
		}
	} else if err == nil {
		err := os.Chmod(mountPath, mountPathPermission)
		if err != nil {
			panic(err)
		}
	}

	mountedFS, err := fuse.Mount(mountPath, server, cfg)
	if err != nil {
		fmt.Printf("Error mounting filesystem %s\n", err)
		os.Exit(1)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		for {
			<-sigChan
			err := fuse.Unmount(mountPath)
			if err != nil {
				fmt.Printf("Unmount error %v\n", err)
			} else {
				break
			}
		}
	}()

	go func() {
		if args.UnmountAfter > 0 {
			time.Sleep(time.Second * time.Duration(args.UnmountAfter))
			err := fuse.Unmount(mountPath)
			if err != nil {
				fmt.Printf("Unmount error %v\n", err)
			}
		}
	}()

	err = mountedFS.Join(context.Background())
	if err != nil {
		fmt.Printf("Error unmounting filesystem %s\n", err)
		os.Exit(1)
	}
}

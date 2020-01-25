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
	unmountSleep = 5
	version = "0.1.1"
)

type args struct {
	ContentFiles bool `default:"true" arg:"-C"`
	CreateMountPath bool   `default:"true" arg:"-c"`
	FirstLineFiles bool `default:"false" arg:"-f"`
	MountPath       string `default:"$HOME/.mnt/passfuse" arg:"-m"`
	PasswordStorePath string `arg:"-s"`
	Prefix string `arg:"-p"`
	UnmountAfter int `arg:"-u"`
}

func (args) Version() string {
	return version
}

func unmount(mountPath string) {
	for {
		err := fuse.Unmount(mountPath)
		if err != nil {
			fmt.Printf("Unmount error %v, sleeping for %d seconds\n", err, unmountSleep)
			time.Sleep(time.Second * unmountSleep)
		} else {
			break
		}
	}
}

func main() {
	args := args{}
	arg.MustParse(&args)

	options := fs.PassFsOptions{ContentFiles:args.ContentFiles, FirstLineFiles:args.FirstLineFiles}
	server, err := fs.NewPassFS(args.PasswordStorePath, args.Prefix, options)
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
			unmount(mountPath)
			break
		}
	}()

	go func() {
		if args.UnmountAfter > 0 {
			time.Sleep(time.Second * time.Duration(args.UnmountAfter))
			unmount(mountPath)
		}
	}()

	err = mountedFS.Join(context.Background())
	if err != nil {
		fmt.Printf("Error serving filesystem %s\n", err)
		os.Exit(1)
	}
}

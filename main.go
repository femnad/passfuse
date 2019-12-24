package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/alexflint/go-arg"
	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"time"
)

func NewHelloFS() (server fuse.Server, err error) {
	user := os.Getuid()
	group := os.Getgid()
	fs := &helloFS{user: uint32(user), group: uint32(group)}

	home := os.Getenv("HOME")
	fi, _ := ioutil.ReadDir(fmt.Sprintf("%s/.password-store", home))
	var children []fuseutil.Dirent
	gInodeInfo = map[fuseops.InodeID]inodeInfo{}
	for i, f := range fi {
		childInode := fuseops.InodeID(fuseops.RootInodeID + 1)
		child := fuseutil.Dirent{
			Offset: fuseops.DirOffset(i + 1),
			Inode:  childInode,
			Name:   f.Name(),
			Type:   getType(f),
		}
		children = append(children, child)
		gInodeInfo[childInode] = inodeInfo{
			attributes: fuseops.InodeAttributes{
				Nlink: 1,
				Mode:  getMode(f),
				Uid:   fs.user,
				Gid:   fs.group,
			},
			dir:      f.IsDir(),
			children: nil,
		}
	}
	gInodeInfo[fuseops.RootInodeID] = inodeInfo{
		attributes: fuseops.InodeAttributes{
			Nlink: 1,
			Mode:  0555 | os.ModeDir,
		},
		dir:      true,
		children: children,
	}
	server = fuseutil.NewFileSystemServer(fs)
	return
}

type helloFS struct {
	fuseutil.NotImplementedFileSystem
	user  uint32
	group uint32
}

const (
	rootInode fuseops.InodeID = fuseops.RootInodeID + iota
	helloInode
	dirInode
	worldInode
)

type inodeInfo struct {
	attributes fuseops.InodeAttributes

	// File or directory?
	dir bool

	// For directories, children.
	children []fuseutil.Dirent
}

// We have a fixed directory structure.
var gInodeInfo map[fuseops.InodeID]inodeInfo

func findChildInode(
	name string,
	children []fuseutil.Dirent) (inode fuseops.InodeID, err error) {
	for _, e := range children {
		if e.Name == name {
			inode = e.Inode
			return
		}
	}

	err = fuse.ENOENT
	return
}

func (fs *helloFS) patchAttributes(
	attr *fuseops.InodeAttributes) {
	now := time.Now()
	attr.Atime = now
	attr.Mtime = now
	attr.Crtime = now
	attr.Uid = fs.user
	attr.Gid = fs.group
}

func (fs *helloFS) StatFS(
	ctx context.Context,
	op *fuseops.StatFSOp) (err error) {
	return
}

func getType(info os.FileInfo) fuseutil.DirentType {
	if info.IsDir() {
		return fuseutil.DT_Directory
	}
	return fuseutil.DT_File
}

func getMode(info os.FileInfo) os.FileMode {
	if info.IsDir() {
		return 0555 | os.ModeDir
	}
	return 0644
}

func (fs *helloFS) LookUpInode(
	ctx context.Context,
	op *fuseops.LookUpInodeOp) (err error) {
	// Find the info for the parent.
	parentInfo, ok := gInodeInfo[op.Parent]
	if !ok {
		err = fuse.ENOENT
		return
	}

	// Find the child within the parent.
	childInode, err := findChildInode(op.Name, parentInfo.children)
	if err != nil {
		return
	}

	// Copy over information.
	op.Entry.Child = childInode
	op.Entry.Attributes = gInodeInfo[childInode].attributes

	// Patch attributes.
	fs.patchAttributes(&op.Entry.Attributes)

	return
}

func (fs *helloFS) GetInodeAttributes(
	ctx context.Context,
	op *fuseops.GetInodeAttributesOp) (err error) {
	// Find the info for this inode.
	info, ok := gInodeInfo[op.Inode]
	if !ok {
		err = fuse.ENOENT
		return
	}

	// Copy over its attributes.
	op.Attributes = info.attributes

	// Patch attributes.
	fs.patchAttributes(&op.Attributes)

	return
}

func (fs *helloFS) OpenDir(
	ctx context.Context,
	op *fuseops.OpenDirOp) (err error) {
	// Allow opening any directory.
	return
}

func (fs *helloFS) ReadDir(
	ctx context.Context,
	op *fuseops.ReadDirOp) (err error) {
	// Find the info for this inode.
	info, ok := gInodeInfo[op.Inode]
	if !ok {
		err = fuse.ENOENT
		return
	}

	if !info.dir {
		err = fuse.EIO
		return
	}

	entries := info.children

	// Grab the range of interest.
	if op.Offset > fuseops.DirOffset(len(entries)) {
		err = fuse.EIO
		return
	}

	entries = entries[op.Offset:]

	// Resume at the specified offset into the array.
	for _, e := range entries {
		n := fuseutil.WriteDirent(op.Dst[op.BytesRead:], e)
		if n == 0 {
			break
		}

		op.BytesRead += n
	}

	return
}

func (fs *helloFS) OpenFile(
	ctx context.Context,
	op *fuseops.OpenFileOp) (err error) {
	// Allow opening any file.
	return
}

func (fs *helloFS) ReadFile(
	ctx context.Context,
	op *fuseops.ReadFileOp) (err error) {
	// Let io.ReaderAt deal with the semantics.
	reader := strings.NewReader("Hello, world!")

	op.BytesRead, err = reader.ReadAt(op.Dst, op.Offset)

	// Special case: FUSE doesn't expect us to return io.EOF.
	if err == io.EOF {
		err = nil
	}

	return
}

var args struct {
	MountPath       string `default:"/tmp/.passfuse"`
	CreateMountPath bool   `default:"true"`
}

func main() {
	arg.MustParse(&args)
	server, _ := NewHelloFS()
	cfg := &fuse.MountConfig{}
	_, err := os.Stat(args.MountPath)
	if errors.Is(err, os.ErrNotExist) && args.CreateMountPath {
		err = os.MkdirAll(args.MountPath, 0755)
		if err != nil {
			panic(err)
		}
	}
	fs, err := fuse.Mount(args.MountPath, server, cfg)
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
	err = fs.Join(context.Background())
	if err != nil {
		panic(err)
	}
}

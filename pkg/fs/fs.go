package fs

import (
	"context"
	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"time"
)

const defaultPath = "$HOME/.password-store"

func NewPassFS(path string) (server fuse.Server, err error) {
	user := uint32(os.Getuid())
	group := uint32(os.Getgid())

	if path == "" {
		path = os.ExpandEnv(defaultPath)
	}

	fi, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var children []fuseutil.Dirent
	inodes := make(map[fuseops.InodeID]inodeInfo)
	for i, f := range fi {
		childInode := fuseops.InodeID(fuseops.RootInodeID + 1)
		child := fuseutil.Dirent{
			Offset: fuseops.DirOffset(i + 1),
			Inode:  childInode,
			Name:   f.Name(),
			Type:   getType(f),
		}
		children = append(children, child)
		inodes[childInode] = inodeInfo{
			attributes: fuseops.InodeAttributes{
				Nlink: 1,
				Mode:  getMode(f),
				Uid:   user,
				Gid:   group,
			},
			dir:      f.IsDir(),
			children: nil,
		}
	}
	inodes[fuseops.RootInodeID] = inodeInfo{
		attributes: fuseops.InodeAttributes{
			Nlink: 1,
			Mode:  0555 | os.ModeDir,
		},
		dir:      true,
		children: children,
	}
	fs := &passFS{inodes: inodes, user: user, group: group}
	server = fuseutil.NewFileSystemServer(fs)
	return
}

type passFS struct {
	fuseutil.NotImplementedFileSystem
	user  uint32
	group uint32
	inodes map[fuseops.InodeID]inodeInfo
}

type inodeInfo struct {
	attributes fuseops.InodeAttributes

	// File or directory?
	dir bool

	// For directories, children.
	children []fuseutil.Dirent
}

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

func (fs *passFS) patchAttributes(
	attr *fuseops.InodeAttributes) {
	now := time.Now()
	attr.Atime = now
	attr.Mtime = now
	attr.Crtime = now
	attr.Uid = fs.user
	attr.Gid = fs.group
}

func (fs *passFS) StatFS(
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

func (fs *passFS) LookUpInode(
	ctx context.Context,
	op *fuseops.LookUpInodeOp) (err error) {
	// Find the info for the parent.
	parentInfo, ok := fs.inodes[op.Parent]
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
	op.Entry.Attributes = fs.inodes[childInode].attributes

	// Patch attributes.
	fs.patchAttributes(&op.Entry.Attributes)

	return
}

func (fs *passFS) GetInodeAttributes(
	ctx context.Context,
	op *fuseops.GetInodeAttributesOp) (err error) {
	// Find the info for this inode.
	info, ok := fs.inodes[op.Inode]
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

func (fs *passFS) OpenDir(
	ctx context.Context,
	op *fuseops.OpenDirOp) (err error) {
	// Allow opening any directory.
	return
}

func (fs *passFS) ReadDir(
	ctx context.Context,
	op *fuseops.ReadDirOp) (err error) {
	// Find the info for this inode.
	info, ok := fs.inodes[op.Inode]
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

func (fs *passFS) OpenFile(
	ctx context.Context,
	op *fuseops.OpenFileOp) (err error) {
	// Allow opening any file.
	return
}

func (fs *passFS) ReadFile(
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

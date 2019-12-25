package fs

import (
	"context"
	"fmt"
	"github.com/femnad/passfuse/pkg/pass"
	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
	"io"
	"os"
	"path"
	"strings"
	"time"
)

const (
	dirSize = 4096
	maxSize = 1024
)

func (fs *passFS) getChildren(root *pass.Node) []fuseutil.Dirent {
	var children []fuseutil.Dirent
	if !root.IsLeaf {
		for index, child := range root.Children {
			childInode := fs.allocateInode()
			childEntry := fuseutil.Dirent{
				Offset: fuseops.DirOffset(index + 1),
				Inode:  childInode,
				Name:   child.Secret,
				Type:   getType(child),
			}
			childSize := dirSize
			if child.IsLeaf {
				childSize = maxSize
			}
			children = append(children, childEntry)
			fs.inodes[childInode] = inodeInfo{
				attributes: fuseops.InodeAttributes{
					Nlink: 1,
					Mode:  getMode(child),
					Uid:   fs.user,
					Gid:   fs.group,
					Size:uint64(childSize),
				},
				dir:      !child.IsLeaf,
				children: fs.getChildren(&child),
				secret: path.Join(root.Secret, child.Secret),
			}
		}
	}
	rootSize := dirSize
	if root.IsLeaf {
		rootSize = maxSize
	}
	fs.inodes[fs.allocateInode()] = inodeInfo{
		attributes: fuseops.InodeAttributes{
			Nlink: 1,
			Mode:  getMode(*root),
			Uid:   fs.user,
			Gid:   fs.group,
			Size:uint64(rootSize),
		},
		dir:        !root.IsLeaf,
		children:   children,
		secret:root.Secret,
	}
	return children
}

func (fs *passFS) allocateInode() fuseops.InodeID {
	allocatedInode := fs.allocatableInode
	fs.allocatableInode++
	return allocatedInode
}

func NewPassFS(path string) (server fuse.Server, err error) {
	user := uint32(os.Getuid())
	group := uint32(os.Getgid())

	rootNode, err := pass.GetPassTree(path)
	if err != nil {
		return nil, err
	}

	inodes := make(map[fuseops.InodeID]inodeInfo)
	fs := &passFS{inodes: inodes, user: user, group: group, allocatableInode:fuseops.RootInodeID+1}
	inodes[fuseops.RootInodeID] = inodeInfo{
		attributes: fuseops.InodeAttributes{
			Nlink: 1,
			Mode:  0555 | os.ModeDir,
		},
		dir:      true,
		children: fs.getChildren(&rootNode),
	}
	server = fuseutil.NewFileSystemServer(fs)
	return
}

type passFS struct {
	fuseutil.NotImplementedFileSystem
	user  uint32
	group uint32
	inodes map[fuseops.InodeID]inodeInfo
	allocatableInode fuseops.InodeID
}

type inodeInfo struct {
	attributes fuseops.InodeAttributes

	// File or directory?
	dir bool

	// For directories, children.
	children []fuseutil.Dirent

	secret string
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

func getType(node pass.Node) fuseutil.DirentType {
	if node.IsLeaf {
		return fuseutil.DT_File
	}
	return fuseutil.DT_Directory
}

func getMode(node pass.Node) os.FileMode {
	if node.IsLeaf {
		return 0644
	}
	return 0555 | os.ModeDir
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

func (fs *passFS) ReadFile(ctx context.Context, op *fuseops.ReadFileOp) (err error) {
	inode, err := fs.getInode(op.Inode)
	if err != nil {
		return err
	}

	secretContent, err := pass.GetSecret(inode.secret)

	// Let io.ReaderAt deal with the semantics.
	reader := strings.NewReader(secretContent)

	op.BytesRead, err = reader.ReadAt(op.Dst, op.Offset)

	// Special case: FUSE doesn't expect us to return io.EOF.
	if err == io.EOF {
		err = nil
	}

	return
}

func (fs *passFS) getInode(id fuseops.InodeID) (*inodeInfo, error) {
	inode, ok := fs.inodes[id]
	if !ok {
		return nil, fmt.Errorf("inode %d not found", id)
	}
	return &inode, nil
}

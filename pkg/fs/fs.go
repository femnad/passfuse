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
	"strings"
	"sync"
	"time"
)

const (
	secretFileSuffix = ".gpg"
	secretContentsSuffix = ".contents"
)

func (fs *passFS) allocateInode() fuseops.InodeID {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()
	allocatedInode := fs.allocatableInode
	fs.allocatableInode++
	return allocatedInode
}

func getSecretBaseName(node pass.Node) string {
	splitBySlash := strings.Split(node.Secret, "/")
	if len(splitBySlash) == 0 {
		panic(fmt.Errorf("cannot determine basename of secret %s", node.Secret))
	}
	return splitBySlash[len(splitBySlash)-1]
}

func (fs *passFS) locateChildren(node pass.Node, offset fuseops.DirOffset) fuseutil.Dirent {
	if node.IsLeaf {
		baseName := getSecretBaseName(node)
		displayedName := strings.Replace(baseName, secretFileSuffix, secretContentsSuffix, 1)
		childInode := fs.allocateInode()
		childEnt := fuseutil.Dirent{
			Offset: offset,
			Inode:  childInode,
			Name:   displayedName,
			Type:   fuseutil.DT_File,
		}
		fs.inodes[childInode] = inodeInfo{
			attributes: fuseops.InodeAttributes{
				Nlink: 1,
				Mode:  0644,
			},
			dir: false,
			secret:node.Secret,
		}
		return childEnt
	} else {
		var nodesChildren []fuseutil.Dirent
		for index, child := range node.Children {
			nodesChildren = append(nodesChildren, fs.locateChildren(child, fuseops.DirOffset(index + 1)))
		}
		nodeInode := fs.allocateInode()
		nodeEnt := fuseutil.Dirent{
			Offset: offset,
			Inode:  nodeInode,
			Name:   getSecretBaseName(node),
			Type:   fuseutil.DT_Directory,
		}
		fs.inodes[nodeInode] = inodeInfo{
			attributes: fuseops.InodeAttributes{
				Nlink: 1,
				Mode:  0755 | os.ModeDir,
			},
			dir: true,
			secret:node.Secret,
			children:nodesChildren,
		}
		return nodeEnt
	}
}

func NewPassFS(path string) (server fuse.Server, err error) {
	user := uint32(os.Getuid())
	group := uint32(os.Getgid())

	rootNode, err := pass.GetPassTree(path)
	if err != nil {
		return nil, err
	}

	inodes := make(map[fuseops.InodeID]inodeInfo)
	fs := &passFS{inodes: inodes, user: user, group: group, allocatableInode: fuseops.RootInodeID + 1}
	rootInfo := inodeInfo{
		attributes: fuseops.InodeAttributes{
			Nlink: 1,
			Mode:  0755 | os.ModeDir,
		},
		dir: true,
	}

	var children []fuseutil.Dirent
	for index, child := range rootNode.Children {
		children = append(children, fs.locateChildren(child, fuseops.DirOffset(index+1)))
	}
	rootInfo.children = children
	fs.inodes[fuseops.RootInodeID] = rootInfo
	server = fuseutil.NewFileSystemServer(fs)
	return
}

type passFS struct {
	fuseutil.NotImplementedFileSystem
	user             uint32
	group            uint32
	inodes           map[fuseops.InodeID]inodeInfo
	node             pass.Node
	mutex            sync.Mutex
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
	secretSize, err := pass.GetSecretSize(fs.inodes[childInode].secret)
	if err != nil {
		return err
	}

	op.Entry.Attributes.Size = uint64(secretSize)
	op.Entry.AttributesExpiration = time.Now().Add(time.Hour)

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
	op.AttributesExpiration = time.Now().Add(time.Hour)

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
		if e.Offset == 0 {
			continue
		}
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
	if err != nil {
		return err
	}

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

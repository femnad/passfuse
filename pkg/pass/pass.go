package pass

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
)

const (
	secretSuffix = ".gpg"
	defaultPath = "$HOME/.password-store"
)

type Node struct {
	Children []Node
	Secret   string
	IsLeaf   bool
}

type Parser struct {
	basePath string
}

func (p Parser) GetNodes(root *Node, prefix string) error {
	root.Secret = prefix
	if root.IsLeaf {
		return nil
	}
	nodePath := path.Join(p.basePath, prefix)
	info, err := ioutil.ReadDir(nodePath)
	if err != nil {
		return fmt.Errorf("error reading dir %s: %s", nodePath, err)
	}
	var children []Node
	for _, item := range info {
		if strings.HasPrefix(item.Name(), ".") {
			continue
		}
		childNode := Node{IsLeaf: !item.IsDir()}
		err = p.GetNodes(&childNode, path.Join(prefix, item.Name()))
		if err != nil {
			return err
		}
		children = append(children, childNode)
	}
	root.Children = children
	return nil
}

func GetPassTree(basePath string) (Node, error) {
	if basePath == "" {
		basePath = os.ExpandEnv(defaultPath)
	}
	parser := Parser{basePath:basePath}
	root := Node{IsLeaf: false}
	err := parser.GetNodes(&root, "")
	if err != nil {
		return Node{}, err
	}
	return root, nil
}

func GetSecret(secretName string) (string, error) {
	secretName = strings.TrimSuffix(secretName, secretSuffix)
	cmd := exec.Command("pass", secretName)
	stdout := bytes.Buffer{}
	cmd.Stdout = &stdout
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("error getting secret %s: %s", secretName, err)
	}
	output, err := ioutil.ReadAll(&stdout)
	if err != nil {
		return "", fmt.Errorf("error reading secret %s: %s", secretName, err)
	}
	return string(output), nil
}

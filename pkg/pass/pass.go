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
	defaultPath  = "$HOME/.password-store"
)

type NodeType int

const (
	Contents  NodeType = iota
	FirstLine          = iota
)

type SecretSize struct {
	ContentsSize  uint64
	FirstLineSize uint64
}

type Node struct {
	Children []Node
	IsLeaf   bool
	Secret   string
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

	singleSecretPrefix := fmt.Sprintf("%s%s", nodePath, secretSuffix)
	_, err := os.Stat(singleSecretPrefix)
	if !os.IsNotExist(err) {
		rootPrefix, _ := path.Split(prefix)
		root.Secret = strings.TrimRight(rootPrefix, "/")
		nodeSecret := fmt.Sprintf("%s%s", prefix, secretSuffix)
		root.Children = []Node{{
			Children: nil,
			IsLeaf:   true,
			Secret:   nodeSecret,
		}}
		root.IsLeaf = false
		return nil
	}

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

func GetPassTree(basePath, prefix string) (Node, error) {
	if basePath == "" {
		basePath = os.ExpandEnv(defaultPath)
	}
	parser := Parser{basePath: basePath}
	root := Node{IsLeaf: false}
	err := parser.GetNodes(&root, prefix)
	if err != nil {
		return Node{}, err
	}
	return root, nil
}

func getSecretContent(secretName string) ([]byte, error) {
	secretName = strings.TrimSuffix(secretName, secretSuffix)
	cmd := exec.Command("pass", secretName)
	stdout := bytes.Buffer{}
	cmd.Stdout = &stdout
	err := cmd.Run()
	if err != nil {
		return []byte{}, fmt.Errorf("error getting secret %s: %s", secretName, err)
	}
	output, err := ioutil.ReadAll(&stdout)
	return output, err
}

func GetSecret(secretName string) (string, error) {
	output, err := getSecretContent(secretName)
	if err != nil {
		return "", fmt.Errorf("error reading secret %s: %s", secretName, err)
	}
	return string(output), nil
}

func GetFirstLine(secretBody string) (string, error) {
	lines := strings.Split(secretBody, "\n")
	if len(lines) == 0 {
		return "", fmt.Errorf("couldn't find any lines in secret body")
	}
	return lines[0], nil
}

func GetSecretSize(secretName string) (secretSize SecretSize, err error) {
	secretBody, err := GetSecret(secretName)
	if err != nil {
		return secretSize, fmt.Errorf("error getting secret body for %s: %s", secretName, err)
	}
	contentsSize := len(secretBody)

	firstLine, err := GetFirstLine(secretBody)
	if err != nil {
		return secretSize, fmt.Errorf("error determining first line for secret %s: %s", secretName, err)
	}
	firstLineSize := len(firstLine)

	secretSize = SecretSize{
		ContentsSize:  uint64(contentsSize),
		FirstLineSize: uint64(firstLineSize),
	}

	return
}

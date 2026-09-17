package main

// FileAssociation 是当前用户注册的文件打开命令。
type FileAssociation struct {
	Extension string `json:"extension"`
	Command   string `json:"command"`
	Active    bool   `json:"active"`
}

type FileAssociationService struct{}

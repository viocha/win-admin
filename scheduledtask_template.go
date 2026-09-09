package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var scheduledTaskTemplateMu sync.Mutex

func (s *ScheduledTaskTemplateService) List() ([]ScheduledTaskTemplate, error) {
	scheduledTaskTemplateMu.Lock()
	defer scheduledTaskTemplateMu.Unlock()

	templates, err := readScheduledTaskTemplates()
	if err != nil {
		return nil, err
	}
	sort.SliceStable(templates, func(i, j int) bool {
		return strings.ToLower(templates[i].Name) < strings.ToLower(templates[j].Name)
	})
	return templates, nil
}

func (s *ScheduledTaskTemplateService) Save(template ScheduledTaskTemplate) (ScheduledTaskTemplate, error) {
	scheduledTaskTemplateMu.Lock()
	defer scheduledTaskTemplateMu.Unlock()

	template.Name = strings.TrimSpace(template.Name)
	if template.Name == "" {
		return ScheduledTaskTemplate{}, errors.New("模板名称不能为空")
	}

	task, err := validateScheduledTaskEdit(template.Task)
	if err != nil {
		return ScheduledTaskTemplate{}, err
	}
	template.Task = task

	templates, err := readScheduledTaskTemplates()
	if err != nil {
		return ScheduledTaskTemplate{}, err
	}
	for _, current := range templates {
		if strings.EqualFold(current.Name, template.Name) && current.ID != template.ID {
			return ScheduledTaskTemplate{}, errors.New("模板名称已存在")
		}
	}

	if strings.TrimSpace(template.ID) == "" {
		template.ID, err = newScheduledTaskTemplateID()
		if err != nil {
			return ScheduledTaskTemplate{}, err
		}
	}

	updated := false
	for index := range templates {
		if templates[index].ID == template.ID {
			templates[index] = template
			updated = true
			break
		}
	}
	if !updated {
		templates = append(templates, template)
	}

	if err := writeScheduledTaskTemplates(templates); err != nil {
		return ScheduledTaskTemplate{}, err
	}
	return template, nil
}

func (s *ScheduledTaskTemplateService) Delete(id string) error {
	scheduledTaskTemplateMu.Lock()
	defer scheduledTaskTemplateMu.Unlock()

	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("模板 ID 不能为空")
	}
	templates, err := readScheduledTaskTemplates()
	if err != nil {
		return err
	}

	next := templates[:0]
	found := false
	for _, template := range templates {
		if template.ID == id {
			found = true
			continue
		}
		next = append(next, template)
	}
	if !found {
		return errors.New("模板不存在")
	}
	return writeScheduledTaskTemplates(next)
}

func scheduledTaskTemplatePath() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("读取用户配置目录失败: %w", err)
	}
	return filepath.Join(root, "win-admin", "scheduled-task-templates.json"), nil
}

func readScheduledTaskTemplates() ([]ScheduledTaskTemplate, error) {
	path, err := scheduledTaskTemplatePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []ScheduledTaskTemplate{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取计划任务模板失败: %w", err)
	}
	if len(data) == 0 {
		return []ScheduledTaskTemplate{}, nil
	}
	var templates []ScheduledTaskTemplate
	if err := json.Unmarshal(data, &templates); err != nil {
		return nil, fmt.Errorf("解析计划任务模板失败: %w", err)
	}
	return templates, nil
}

func writeScheduledTaskTemplates(templates []ScheduledTaskTemplate) error {
	path, err := scheduledTaskTemplatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建模板目录失败: %w", err)
	}
	data, err := json.MarshalIndent(templates, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化计划任务模板失败: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("保存计划任务模板失败: %w", err)
	}
	return nil
}

func newScheduledTaskTemplateID() (string, error) {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("生成模板 ID 失败: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

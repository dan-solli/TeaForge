package tools

import (
	"encoding/json"
	"fmt"

	"github.com/dan-solli/teaforge/internal/prompt"
)

type coreToolTexts struct {
	ReadFileDescription                  string `json:"read_file_description"`
	ReadFilePathParamDescription         string `json:"read_file_path_param_description"`
	WriteFileDescription                 string `json:"write_file_description"`
	WriteFilePathParamDescription        string `json:"write_file_path_param_description"`
	WriteFileContentParamDescription     string `json:"write_file_content_param_description"`
	EditFileDescription                  string `json:"edit_file_description"`
	EditFilePathParamDescription         string `json:"edit_file_path_param_description"`
	EditFileOldStrParamDescription       string `json:"edit_file_old_str_param_description"`
	EditFileNewStrParamDescription       string `json:"edit_file_new_str_param_description"`
	ListDirectoryDescription             string `json:"list_directory_description"`
	ListDirectoryPathParamDescription    string `json:"list_directory_path_param_description"`
	RunCommandDescription                string `json:"run_command_description"`
	RunCommandCommandParamDescription    string `json:"run_command_command_param_description"`
	RunCommandWorkingDirParamDescription string `json:"run_command_working_dir_param_description"`
}

var toolText = mustLoadCoreToolTexts()

func mustLoadCoreToolTexts() coreToolTexts {
	raw := prompt.MustLoadTemplate("core_tool_texts.json")
	var texts coreToolTexts
	if err := json.Unmarshal([]byte(raw), &texts); err != nil {
		panic(fmt.Errorf("parse embedded core tool texts: %w", err))
	}
	return texts
}

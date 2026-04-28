package agent

import (
	"encoding/json"
	"fmt"

	"github.com/dan-solli/teaforge/internal/prompt"
)

type agentToolTexts struct {
	SaveNoteDescription                 string `json:"save_note_description"`
	SaveNoteCategoryParamDescription    string `json:"save_note_category_param_description"`
	SaveNoteContentParamDescription     string `json:"save_note_content_param_description"`
	RecallNotesDescription              string `json:"recall_notes_description"`
	RecallNotesQueryParamDescription    string `json:"recall_notes_query_param_description"`
	RecallNotesCategoryParamDescription string `json:"recall_notes_category_param_description"`
	ListNoteCategoriesDescription       string `json:"list_note_categories_description"`
	SearchCodeDescription               string `json:"search_code_description"`
	SearchCodeQueryParamDescription     string `json:"search_code_query_param_description"`
	IndexDirectoryDescription           string `json:"index_directory_description"`
	IndexDirectoryPathParamDescription  string `json:"index_directory_path_param_description"`
}

var agentToolText = mustLoadAgentToolTexts()

func mustLoadAgentToolTexts() agentToolTexts {
	raw := prompt.MustLoadTemplate("agent_tool_texts.json")
	var texts agentToolTexts
	if err := json.Unmarshal([]byte(raw), &texts); err != nil {
		panic(fmt.Errorf("parse embedded agent tool texts: %w", err))
	}
	return texts
}

package comments

import "encoding/json"

type commentPayload struct {
	ID          string        `json:"id"`
	AuthorName  string        `json:"author_name"`
	Author      commentAuthor `json:"author"`
	Content     string        `json:"content"`
	Type        string        `json:"type"`
	CommentType string        `json:"comment_type"`
	CreatedAt   string        `json:"created_at"`
	DeletedAt   string        `json:"deleted_at"`
}

type commentAuthor struct {
	ID    string
	Name  string
	Email string
}

func (a *commentAuthor) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}

	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		a.Name = name
		return nil
	}

	var obj struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	a.ID = obj.ID
	a.Name = obj.Name
	a.Email = obj.Email
	return nil
}

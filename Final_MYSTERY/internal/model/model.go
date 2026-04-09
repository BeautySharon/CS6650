package model

type Album struct {
	AlbumID     string `json:"album_id" dynamodbav:"album_id"`
	Title       string `json:"title" dynamodbav:"title"`
	Description string `json:"description" dynamodbav:"description"`
	Owner       string `json:"owner" dynamodbav:"owner"`
	CreatedAt   string `json:"created_at,omitempty" dynamodbav:"created_at"`
	UpdatedAt   string `json:"updated_at,omitempty" dynamodbav:"updated_at"`
}

type Photo struct {
	PhotoID    string `json:"photo_id" dynamodbav:"photo_id"`
	AlbumID    string `json:"album_id" dynamodbav:"album_id"`
	Seq        int64  `json:"seq" dynamodbav:"seq"`
	Status     string `json:"status" dynamodbav:"status"`
	StagingKey string `json:"-" dynamodbav:"staging_key"`
	FinalKey   string `json:"-" dynamodbav:"final_key"`
	URL        string `json:"url,omitempty" dynamodbav:"url"`
	CreatedAt  string `json:"created_at,omitempty" dynamodbav:"created_at"`
	UpdatedAt  string `json:"updated_at,omitempty" dynamodbav:"updated_at"`
}

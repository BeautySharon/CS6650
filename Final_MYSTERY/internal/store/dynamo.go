package store

import (
	"context"
	"errors"
	"strconv"
	"time"

	"album-store/internal/model"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var ErrNotFound = errors.New("not found")

type DynamoStore struct {
	db            *dynamodb.Client
	albumsTable   string
	photosTable   string
	albumSeqTable string
}

func NewDynamoStore(db *dynamodb.Client, albums, photos, albumSeq string) *DynamoStore {
	return &DynamoStore{db: db, albumsTable: albums, photosTable: photos, albumSeqTable: albumSeq}
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func (s *DynamoStore) PutAlbum(ctx context.Context, a model.Album) error {
	ts := now()
	if a.CreatedAt == "" {
		a.CreatedAt = ts
	}
	a.UpdatedAt = ts
	item, err := attributevalue.MarshalMap(a)
	if err != nil {
		return err
	}
	_, err = s.db.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(s.albumsTable), Item: item})
	return err
}

func (s *DynamoStore) GetAlbum(ctx context.Context, albumID string) (model.Album, error) {
	out, err := s.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.albumsTable),
		Key:       map[string]types.AttributeValue{"album_id": &types.AttributeValueMemberS{Value: albumID}},
	})
	if err != nil {
		return model.Album{}, err
	}
	if len(out.Item) == 0 {
		return model.Album{}, ErrNotFound
	}
	var a model.Album
	if err := attributevalue.UnmarshalMap(out.Item, &a); err != nil {
		return model.Album{}, err
	}
	return a, nil
}

// ListAlbums fetches all albums via paginated scan.
func (s *DynamoStore) ListAlbums(ctx context.Context) ([]model.Album, error) {
	var outAlbums []model.Album
	var startKey map[string]types.AttributeValue
	for {
		out, err := s.db.Scan(ctx, &dynamodb.ScanInput{
			TableName:         aws.String(s.albumsTable),
			ExclusiveStartKey: startKey,
		})
		if err != nil {
			return nil, err
		}
		var page []model.Album
		if err := attributevalue.UnmarshalListOfMaps(out.Items, &page); err != nil {
			return nil, err
		}
		outAlbums = append(outAlbums, page...)
		if len(out.LastEvaluatedKey) == 0 {
			break
		}
		startKey = out.LastEvaluatedKey
	}
	return outAlbums, nil
}

func (s *DynamoStore) NextSeq(ctx context.Context, albumID string) (int64, error) {
	out, err := s.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.albumSeqTable),
		Key: map[string]types.AttributeValue{
			"album_id": &types.AttributeValueMemberS{Value: albumID},
		},
		UpdateExpression: aws.String("ADD next_seq :inc"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":inc": &types.AttributeValueMemberN{Value: "1"},
		},
		ReturnValues: types.ReturnValueUpdatedNew,
	})
	if err != nil {
		return 0, err
	}
	attr, ok := out.Attributes["next_seq"].(*types.AttributeValueMemberN)
	if !ok {
		return 0, errors.New("next_seq missing from update result")
	}
	return strconv.ParseInt(attr.Value, 10, 64)
}

func (s *DynamoStore) PutPhoto(ctx context.Context, p model.Photo) error {
	ts := now()
	if p.CreatedAt == "" {
		p.CreatedAt = ts
	}
	p.UpdatedAt = ts
	item, err := attributevalue.MarshalMap(p)
	if err != nil {
		return err
	}
	_, err = s.db.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(s.photosTable), Item: item})
	return err
}

func (s *DynamoStore) GetPhoto(ctx context.Context, photoID string) (model.Photo, error) {
	out, err := s.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.photosTable),
		Key:       map[string]types.AttributeValue{"photo_id": &types.AttributeValueMemberS{Value: photoID}},
	})
	if err != nil {
		return model.Photo{}, err
	}
	if len(out.Item) == 0 {
		return model.Photo{}, ErrNotFound
	}
	var p model.Photo
	if err := attributevalue.UnmarshalMap(out.Item, &p); err != nil {
		return model.Photo{}, err
	}
	return p, nil
}

// MarkPhotoCompleted stores final_key and url. GetPhoto generates a fresh URL
// on read, but the stored url acts as a fallback.
func (s *DynamoStore) MarkPhotoCompleted(ctx context.Context, photoID, finalKey, url string) error {
	_, err := s.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.photosTable),
		Key: map[string]types.AttributeValue{
			"photo_id": &types.AttributeValueMemberS{Value: photoID},
		},
		UpdateExpression: aws.String("SET #status = :status, final_key = :final_key, #url = :url, updated_at = :updated_at"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
			"#url":    "url",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status":     &types.AttributeValueMemberS{Value: "completed"},
			":final_key":  &types.AttributeValueMemberS{Value: finalKey},
			":url":        &types.AttributeValueMemberS{Value: url},
			":updated_at": &types.AttributeValueMemberS{Value: now()},
		},
	})
	return err
}

func (s *DynamoStore) MarkPhotoFailed(ctx context.Context, photoID string) error {
	_, err := s.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                aws.String(s.photosTable),
		Key:                      map[string]types.AttributeValue{"photo_id": &types.AttributeValueMemberS{Value: photoID}},
		UpdateExpression:         aws.String("SET #status = :status, updated_at = :updated_at"),
		ExpressionAttributeNames: map[string]string{"#status": "status"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status":     &types.AttributeValueMemberS{Value: "failed"},
			":updated_at": &types.AttributeValueMemberS{Value: now()},
		},
	})
	return err
}

func (s *DynamoStore) DeletePhoto(ctx context.Context, photoID string) error {
	_, err := s.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.photosTable),
		Key:       map[string]types.AttributeValue{"photo_id": &types.AttributeValueMemberS{Value: photoID}},
	})
	return err
}

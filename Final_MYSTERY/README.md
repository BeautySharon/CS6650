# Album Store starter

This is a starter implementation for the **CS 6650 ChaosArena Contract v1: Album Store** assignment. It includes:

- Go API service
- Go worker service
- AWS S3 + SQS + DynamoDB integration
- Terraform for VPC, ALB, ECS Fargate, DynamoDB, S3, and SQS

## What is already implemented

- `GET /health`
- `PUT /albums/:album_id`
- `GET /albums/:album_id`
- `GET /albums`
- `POST /albums/:album_id/photos` returning `202`
- `GET /albums/:album_id/photos/:photo_id`
- `DELETE /albums/:album_id/photos/:photo_id`
- Per-album atomic `seq` assignment using DynamoDB
- Background worker using SQS
- Presigned S3 URL generation for completed photos

## Important note

This folder is designed to be a strong starter and deployment scaffold. I could not actually deploy it into your AWS account from here, so you should still run:

- `go build ./...`
- `terraform init && terraform apply`
- end-to-end curl tests
- a real ChaosArena submission

## Local build

```bash
go mod tidy
go build ./...
```

## Build Docker images

```bash
docker build -f Dockerfile.api -t album-store-api:latest .
docker build -f Dockerfile.worker -t album-store-worker:latest .
```

## Push images to ECR

Create two ECR repos, then tag and push.

```bash
aws ecr create-repository --repository-name album-store-api
aws ecr create-repository --repository-name album-store-worker

AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
AWS_REGION=us-west-2

aws ecr get-login-password --region $AWS_REGION | \
  docker login --username AWS --password-stdin ${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com

docker tag album-store-api:latest ${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com/album-store-api:latest
docker tag album-store-worker:latest ${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com/album-store-worker:latest

docker push ${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com/album-store-api:latest
docker push ${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com/album-store-worker:latest
```

## Deploy infra

From the `terraform/` folder:

```bash
terraform init
terraform apply \
  -var="aws_region=us-west-2" \
  -var="api_image=YOUR_API_ECR_IMAGE_URI" \
  -var="worker_image=YOUR_WORKER_ECR_IMAGE_URI"
```

Then get the output:

```bash
terraform output base_url
```

## Smoke tests

Set:

```bash
BASE_URL=http://your-alb-dns-name
ALBUM_ID=$(uuidgen | tr '[:upper:]' '[:lower:]')
```

### Health

```bash
curl "$BASE_URL/health"
```

### Create album

```bash
curl -X PUT "$BASE_URL/albums/$ALBUM_ID" \
  -H 'Content-Type: application/json' \
  -d "{
    \"album_id\": \"$ALBUM_ID\",
    \"title\": \"My Summer Trip\",
    \"description\": \"Photos from Cancun\",
    \"owner\": \"student@northeastern.edu\"
  }"
```

### Upload photo

```bash
curl -X POST "$BASE_URL/albums/$ALBUM_ID/photos" \
  -F "photo=@/path/to/test.jpg"
```

### Poll photo status

```bash
PHOTO_ID=replace_me
curl "$BASE_URL/albums/$ALBUM_ID/photos/$PHOTO_ID"
```

### Delete photo

```bash
curl -X DELETE "$BASE_URL/albums/$ALBUM_ID/photos/$PHOTO_ID" -i
```

## Submit to ChaosArena

```bash
curl -X POST https://chaosarena-alb-938452724.us-west-2.elb.amazonaws.com/submit \
  -H "Content-Type: application/json" \
  -d '{
    "email":    "your@northeastern.edu",
    "nickname": "your-nickname",
    "base_url": "http://your-service-url",
    "contract": "v1-album-store"
  }'
```

## Suggested first fixes and improvements

- Return `201` on first album create and `200` on true update
- Add image validation and max upload size guard
- Add a DLQ for the SQS queue
- Add autoscaling for ECS worker service
- Add integration tests
- Add a GSI on `album_id` for easier debugging / future features

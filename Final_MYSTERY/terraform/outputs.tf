output "base_url" {
  value = "http://${aws_lb.api.dns_name}"
}

output "bucket_name" {
  value = aws_s3_bucket.photos.bucket
}

output "albums_table" {
  value = aws_dynamodb_table.albums.name
}

output "photos_table" {
  value = aws_dynamodb_table.photos.name
}

output "album_seq_table" {
  value = aws_dynamodb_table.album_seq.name
}

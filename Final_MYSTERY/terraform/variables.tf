variable "project_name" {
  type    = string
  default = "album-store"
}

variable "aws_region" {
  type    = string
  default = "us-west-2"
}

variable "api_image" {
  type = string
}

variable "worker_image" {
  type = string
}

variable "execution_role_arn" {
  type = string
}

variable "task_role_arn" {
  type = string
}

variable "api_cpu" {
  type    = number
  default = 1024 # 1 vCPU — handles concurrent S3 streaming and Gin request processing
}

variable "api_memory" {
  type    = number
  default = 2048 # 2 GB — headroom for concurrent multipart uploads in memory
}

variable "api_desired_count" {
  type    = number
  default = 3 # run at least 2 tasks for availability and horizontal throughput
}

variable "worker_cpu" {
  type    = number
  default = 1024 # 1 vCPU — serves 16 parallel goroutines doing S3 copy + DynamoDB
}

variable "worker_memory" {
  type    = number
  default = 2048
}

variable "worker_desired_count" {
  type    = number
  default = 2
}

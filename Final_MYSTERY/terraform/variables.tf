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
  default = 512
}

variable "api_memory" {
  type    = number
  default = 1024
}

variable "worker_cpu" {
  type    = number
  default = 512
}

variable "worker_memory" {
  type    = number
  default = 1024
}

variable "worker_desired_count" {
  type    = number
  default = 2
}

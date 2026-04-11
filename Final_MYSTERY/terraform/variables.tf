variable "project_name" {
  type    = string
  default = "album-store"
}

variable "aws_region" {
  type    = string
  default = "us-west-2"
}

variable "api_cpu" {
  type    = number
  default = 4096
}

variable "api_memory" {
  type    = number
  default = 8192
}

variable "api_desired_count" {
  type    = number
  default = 3
}

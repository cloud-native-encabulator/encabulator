# Passwords for the RDS master user and the three application roles. They end up in the
# Secret encabulator-db (kubernetes.tf). No special characters: the chart embeds them in
# DATABASE_URL, and RDS refuses / " @ and spaces in a master password.

resource "random_password" "postgres" {
  length  = 32
  special = false
}

resource "random_password" "app" {
  for_each = toset(local.databases)

  length  = 32
  special = false
}

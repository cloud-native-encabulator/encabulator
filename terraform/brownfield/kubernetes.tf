# The hand-off objects the Helm chart reads (namespace encabulator), and the Job that
# creates the three logical databases and roles on RDS.

resource "kubernetes_namespace_v1" "encabulator" {
  metadata {
    name = local.namespace
    # deploy-app.sh runs `helm upgrade --install --create-namespace`; Helm 4 server-side applies
    # the label name=<namespace> to the namespace even when it already exists. Set it here too,
    # or every plan after the first app deploy shows the label as drift.
    labels = {
      name = local.namespace
    }
  }

  # Auto Mode needs nodes before anything in the namespace can run.
  depends_on = [helm_release.nodes]
}

# -------------------------------------------------------------------- store lookups
# Looked up by name, not read off the managed resources, so the ConfigMap survives the
# Act 1 hand-off unchanged (the managed resources leave this configuration then).

data "aws_db_instance" "pg" {
  db_instance_identifier = local.postgres_lookup_identifier
}

data "aws_elasticache_serverless_cache" "valkey" {
  name = local.valkey_lookup_name
}

# -------------------------------------------------------------------- ConfigMap + Secrets

resource "kubernetes_config_map_v1" "aws" {
  metadata {
    name      = "encabulator-aws"
    namespace = kubernetes_namespace_v1.encabulator.metadata[0].name
  }

  data = {
    POSTGRES_HOST = data.aws_db_instance.pg.address
    POSTGRES_PORT = tostring(data.aws_db_instance.pg.port)
    VALKEY_HOST   = data.aws_elasticache_serverless_cache.valkey.endpoint.address
    VALKEY_PORT   = tostring(data.aws_elasticache_serverless_cache.valkey.endpoint.port)
  }
}

# Key names match the chart's postgres.superuser.passwordKey and
# postgres.databases.<db>.passwordKey, so postgres.external.existingSecret can point here.
resource "kubernetes_secret_v1" "db" {
  metadata {
    name      = "encabulator-db"
    namespace = kubernetes_namespace_v1.encabulator.metadata[0].name
  }

  type = "Opaque"
  data = merge(
    { "postgres-password" = random_password.postgres.result },
    { for db in local.databases : "${db}-password" => random_password.app[db].result },
  )
}

# ElastiCache serverless without a user group takes no password. The chart always reads
# VALKEY_PASSWORD from a Secret, so give it an empty one instead of letting the chart
# generate a password the cache does not know.
resource "kubernetes_secret_v1" "valkey" {
  metadata {
    name      = "encabulator-valkey"
    namespace = kubernetes_namespace_v1.encabulator.metadata[0].name
  }

  type = "Opaque"
  data = {
    "valkey-password" = ""
  }
}

# ------------------------------------------------------------- databases and roles
# RDS is private, so the SQL runs inside the cluster, next to the services that will use
# it: a Job with the PostgreSQL client. The script is idempotent (creates what is missing,
# resets the role passwords to the Secret's values), and Terraform re-runs the Job when the
# script changes.

resource "kubernetes_config_map_v1" "db_init" {
  metadata {
    name      = "encabulator-db-init"
    namespace = kubernetes_namespace_v1.encabulator.metadata[0].name
  }

  data = {
    "init-databases.sql" = file("${path.module}/sql/init-databases.sql")
  }
}

resource "kubernetes_job_v1" "db_init" {
  metadata {
    name      = "encabulator-db-init"
    namespace = kubernetes_namespace_v1.encabulator.metadata[0].name
    labels = {
      "app.kubernetes.io/name"    = "encabulator-db-init"
      "app.kubernetes.io/part-of" = "encabulator"
    }
  }

  spec {
    backoff_limit = 10

    template {
      metadata {
        labels = {
          "app.kubernetes.io/name"    = "encabulator-db-init"
          "app.kubernetes.io/part-of" = "encabulator"
        }
        annotations = {
          # A changed script is a new pod template, so Terraform replaces the Job.
          "encabulator.io/script-sha256" = sha256(kubernetes_config_map_v1.db_init.data["init-databases.sql"])
        }
      }

      spec {
        restart_policy                   = "OnFailure"
        automount_service_account_token  = false
        termination_grace_period_seconds = 10

        security_context {
          run_as_non_root = true
          run_as_user     = 70 # postgres in the Alpine image
          run_as_group    = 70
          seccomp_profile {
            type = "RuntimeDefault"
          }
        }

        container {
          name  = "psql"
          image = "postgres:18.6-alpine3.24@sha256:77f585114c32fbca283dc835b0596f4e52b51b4c6662d7810b2f4084f60a1873"

          command = ["psql"]
          args = [
            "--no-psqlrc",
            "--set=ON_ERROR_STOP=1",
            "--file=/sql/init-databases.sql",
          ]

          env {
            name = "PGHOST"
            value_from {
              config_map_key_ref {
                name = kubernetes_config_map_v1.aws.metadata[0].name
                key  = "POSTGRES_HOST"
              }
            }
          }
          env {
            name = "PGPORT"
            value_from {
              config_map_key_ref {
                name = kubernetes_config_map_v1.aws.metadata[0].name
                key  = "POSTGRES_PORT"
              }
            }
          }
          env {
            name  = "PGUSER"
            value = "postgres"
          }
          env {
            name  = "PGDATABASE"
            value = "postgres"
          }
          env {
            name  = "PGSSLMODE"
            value = "require"
          }
          env {
            name  = "PGCONNECT_TIMEOUT"
            value = "10"
          }
          env {
            name = "PGPASSWORD"
            value_from {
              secret_key_ref {
                name = kubernetes_secret_v1.db.metadata[0].name
                key  = "postgres-password"
              }
            }
          }
          dynamic "env" {
            for_each = toset(local.databases)
            content {
              name = "${upper(env.value)}_PASSWORD"
              value_from {
                secret_key_ref {
                  name = kubernetes_secret_v1.db.metadata[0].name
                  key  = "${env.value}-password"
                }
              }
            }
          }
          env {
            # psql has no history or rc file to write with --no-psqlrc and --file, but
            # keep HOME on a writable volume anyway.
            name  = "HOME"
            value = "/tmp"
          }

          resources {
            requests = {
              cpu    = "50m"
              memory = "64Mi"
            }
            limits = {
              cpu    = "500m"
              memory = "128Mi"
            }
          }

          security_context {
            allow_privilege_escalation = false
            read_only_root_filesystem  = true
            capabilities {
              drop = ["ALL"]
            }
          }

          volume_mount {
            name       = "sql"
            mount_path = "/sql"
            read_only  = true
          }
          volume_mount {
            name       = "tmp"
            mount_path = "/tmp"
          }
        }

        volume {
          name = "sql"
          config_map {
            name = kubernetes_config_map_v1.db_init.metadata[0].name
          }
        }
        volume {
          name = "tmp"
          empty_dir {}
        }
      }
    }
  }

  # Nodes have to come up and RDS has to accept connections; the Job retries meanwhile.
  wait_for_completion = true
  timeouts {
    create = "15m"
    update = "15m"
  }

  depends_on = [aws_vpc_security_group_ingress_rule.db]
}

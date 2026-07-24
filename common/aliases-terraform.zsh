# Terraform aliases compatible with the commonly used Oh My Zsh terraform aliases.

if _selfishell_command_path terraform >/dev/null; then
  alias tf='terraform'
  alias tfa='terraform apply'
  alias tfad='terraform apply -destroy'
  alias tfap='terraform apply -auto-approve'
  alias tfc='terraform console'
  alias tfd='terraform destroy'
  alias tfda='terraform destroy -auto-approve'
  alias tff='terraform fmt'
  alias tffr='terraform fmt -recursive'
  alias tfi='terraform init'
  alias tfiu='terraform init -upgrade'
  alias tfo='terraform output'
  alias tfp='terraform plan'
  alias tfpd='terraform plan -destroy'
  alias tfs='terraform show'
  alias tfv='terraform validate'
  alias tfw='terraform workspace'
  alias tfwl='terraform workspace list'
  alias tfws='terraform workspace select'
  alias tfwn='terraform workspace new'
fi

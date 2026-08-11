# kubectl aliases compatible with the Oh My Zsh kubectl plugin. Completion is
# configured separately in completion.zsh so these aliases have zero startup cost.

if _selfishell_command_path kubectl >/dev/null; then
  alias k='kubectl'
  alias kd='kubectl describe'
  alias kg='kubectl get'
  alias kaf='kubectl apply -f'
  alias keti='kubectl exec -t -i'
  alias kcuc='kubectl config use-context'
  alias kccc='kubectl config current-context'
  alias kcgc='kubectl config get-contexts'
  alias kdel='kubectl delete'
  alias kgp='kubectl get pods'
  alias kgpa='kubectl get pods --all-namespaces'
  alias kgpw='kubectl get pods --watch'
  alias kgpwide='kubectl get pods -o wide'
  alias kdp='kubectl describe pods'
  alias kgs='kubectl get svc'
  alias kgns='kubectl get namespaces'
  alias kcn='kubectl config set-context --current --namespace'
  alias kgd='kubectl get deployment'
  alias krsd='kubectl rollout status deployment'
  alias krrd='kubectl rollout restart deployment'
  alias kpf='kubectl port-forward'
  alias kl='kubectl logs'
  alias klf='kubectl logs -f'
fi

if _selfishell_command_path kubectx >/dev/null; then
  alias kx='kubectx'
fi

if _selfishell_command_path kubens >/dev/null; then
  alias kn='kubens'
fi

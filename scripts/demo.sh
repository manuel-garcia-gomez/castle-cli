#!/usr/bin/env bash
# scripts/demo.sh — Interactive demo of castle-cli without backend services.
#
# Usage:
#   chmod +x scripts/demo.sh
#   ./scripts/demo.sh
#
# Every step pauses and waits for ENTER so you control the pace.
# A temp directory is used as the working directory; it is deleted at exit.

set -euo pipefail

# ── ANSI colours ──────────────────────────────────────────────────────────────
RESET="\033[0m"
BOLD="\033[1m"
DIM="\033[2m"

BLACK="\033[30m"
RED="\033[31m"
GREEN="\033[32m"
YELLOW="\033[33m"
BLUE="\033[34m"
MAGENTA="\033[35m"
CYAN="\033[36m"
WHITE="\033[37m"

BG_BLUE="\033[44m"
BG_MAGENTA="\033[45m"
BG_CYAN="\033[46m"
BG_GREEN="\033[42m"

# ── Helpers ───────────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
BINARY="${REPO_ROOT}/bin/castle"

banner() {
  local bg="$1" text="$2"
  local width=70
  local pad=$(( (width - ${#text}) / 2 ))
  echo ""
  printf "${BOLD}${bg}${BLACK}%${width}s${RESET}\n" ""
  printf "${BOLD}${bg}${BLACK}%${pad}s%s%${pad}s ${RESET}\n" "" "$text" ""
  printf "${BOLD}${bg}${BLACK}%${width}s${RESET}\n" ""
  echo ""
}

step_header() {
  local num="$1" title="$2" subtitle="$3"
  echo ""
  echo -e "${BOLD}${CYAN}┌─────────────────────────────────────────────────────────────────┐${RESET}"
  printf "${BOLD}${CYAN}│${RESET}  ${BOLD}${WHITE}Step %s — %s${RESET}%*s${BOLD}${CYAN}│${RESET}\n" \
    "$num" "$title" $(( 65 - 8 - ${#num} - ${#title} )) ""
  printf "${BOLD}${CYAN}│${RESET}  ${DIM}%s%*s${BOLD}${CYAN}│${RESET}\n" \
    "$subtitle" $(( 65 - 2 - ${#subtitle} )) ""
  echo -e "${BOLD}${CYAN}└─────────────────────────────────────────────────────────────────┘${RESET}"
  echo ""
}

run_cmd() {
  echo -e "${BOLD}${GREEN}\$${RESET} ${BOLD}$*${RESET}"
  echo ""
  "$@"
}

show_file() {
  local label="$1" path="$2"
  echo -e "${BOLD}${YELLOW}── ${label} ──────────────────────────────────────────────────────────${RESET}"
  # Syntax-highlight YAML keys in cyan, values in white
  while IFS= read -r line; do
    if [[ "$line" =~ ^([[:space:]]*[a-zA-Z_-]+:)(.*)$ ]]; then
      printf "${CYAN}%s${RESET}%s\n" "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}"
    elif [[ "$line" =~ ^--- ]]; then
      echo -e "${DIM}${line}${RESET}"
    elif [[ "$line" =~ ^# ]]; then
      echo -e "${DIM}${GREEN}${line}${RESET}"
    else
      echo "$line"
    fi
  done < "$path"
  echo -e "${BOLD}${YELLOW}────────────────────────────────────────────────────────────────────${RESET}"
}

pause() {
  echo ""
  echo -e "${DIM}Press ${BOLD}ENTER${RESET}${DIM} to continue…${RESET}"
  read -r
}

info() {
  echo -e "${MAGENTA}ℹ  $*${RESET}"
}

success() {
  echo -e "${BOLD}${GREEN}✔  $*${RESET}"
}

# ── Build check ───────────────────────────────────────────────────────────────
check_binary() {
  if [[ ! -x "${BINARY}" ]]; then
    echo -e "${YELLOW}⚙  Binary not found at ${BINARY}. Building…${RESET}"
    echo ""
    (cd "${REPO_ROOT}" && make build)
    echo ""
  fi
  success "Binary ready: ${BINARY}"
}

# ── Temp working directory ────────────────────────────────────────────────────
WORK_DIR=""

setup_workdir() {
  WORK_DIR="$(mktemp -d -t castle-demo-XXXXXX)"
  echo -e "${DIM}Working directory: ${WORK_DIR}${RESET}"
}

cleanup() {
  if [[ -n "${WORK_DIR}" && -d "${WORK_DIR}" ]]; then
    rm -rf "${WORK_DIR}"
    echo ""
    echo -e "${DIM}Temporary directory removed.${RESET}"
  fi
}
trap cleanup EXIT

# ═════════════════════════════════════════════════════════════════════════════
# MAIN
# ═════════════════════════════════════════════════════════════════════════════

banner "${BG_BLUE}" "  Castle CLI — Interactive Technical Demo  "

echo -e "${BOLD}${WHITE}What you will see:${RESET}"
echo -e "  ${CYAN}1.${RESET} Zero-Trust scaffolding   (castle init)"
echo -e "  ${CYAN}2.${RESET} Shift-Left security scan  (castle scan)"
echo -e "  ${CYAN}3.${RESET} GitOps deployment          (castle deploy)"
echo ""
info "No backend services (ArgoCD, DefectDojo, Trivy) are required."
info "All steps run in an isolated temp directory; nothing is committed."

pause

# ── 0. Setup ──────────────────────────────────────────────────────────────────
check_binary
setup_workdir
cd "${WORK_DIR}"

# ═════════════════════════════════════════════════════════════════════════════
# STEP 1 — Zero-Trust: castle init
# ═════════════════════════════════════════════════════════════════════════════
step_header "1" "Zero-Trust Scaffolding" \
  "Generate Istio + OPA Gatekeeper manifests for checkout-service"

info "castle init creates three manifests under infra/<app>/:"
echo -e "  ${DIM}namespace.yaml            — Namespace with Istio injection enabled${RESET}"
echo -e "  ${DIM}peer-authentication.yaml  — mTLS STRICT PeerAuthentication${RESET}"
echo -e "  ${DIM}gatekeeper.yaml           — OPA ConstraintTemplate + K8sRequiredLabels${RESET}"
echo ""

run_cmd "${BINARY}" init --app checkout-service --team platform

echo ""
show_file "infra/checkout-service/peer-authentication.yaml" \
  "infra/checkout-service/peer-authentication.yaml"

echo ""
success "Zero-Trust manifests ready to commit to your GitOps repository."

pause

# ═════════════════════════════════════════════════════════════════════════════
# STEP 2 — Shift-Left: castle scan
# ═════════════════════════════════════════════════════════════════════════════
step_header "2" "Shift-Left Security Scan" \
  "Run Trivy in stub/fallback mode (no Trivy binary required)"

info "castle scan detects whether Trivy is available in \$PATH."
if command -v trivy &>/dev/null; then
  info "Trivy found at $(command -v trivy) — running real scan."
else
  echo -e "  ${YELLOW}⚠  Trivy not found in \$PATH.${RESET}"
  echo -e "  ${DIM}castle scan falls back to a stub JSON report so the pipeline${RESET}"
  echo -e "  ${DIM}continues and the manifest is created for review.${RESET}"
fi
echo ""

run_cmd "${BINARY}" scan --type trivy --target .

echo ""
success "Scan report written. In a real pipeline this would be uploaded to DefectDojo."

pause

# ═════════════════════════════════════════════════════════════════════════════
# STEP 3 — GitOps: castle deploy
# ═════════════════════════════════════════════════════════════════════════════
step_header "3" "GitOps Deployment" \
  "Generate an ArgoCD Application manifest for checkout-service → staging"

info "castle deploy writes the ArgoCD Application YAML to infra/<app>/argocd-app.yaml."
info "Without --sync the manifest is only written locally; commit it to trigger ArgoCD."
echo ""

run_cmd "${BINARY}" deploy \
  --app checkout-service \
  --env staging \
  --repo https://github.com/org/gitops.git

echo ""
show_file "infra/checkout-service/argocd-app.yaml" \
  "infra/checkout-service/argocd-app.yaml"

echo ""
success "ArgoCD manifest ready. Add --sync (with argocd.url + argocd.token configured)"
echo -e "  ${DIM}to trigger an immediate sync via the ArgoCD REST API.${RESET}"

# ═════════════════════════════════════════════════════════════════════════════
# WRAP-UP
# ═════════════════════════════════════════════════════════════════════════════
banner "${BG_GREEN}" "  Demo complete — castle-cli covers Init → Scan → Deploy  "

echo -e "${BOLD}${WHITE}Key takeaways:${RESET}"
echo -e "  ${GREEN}✔${RESET} Zero-Trust Kubernetes manifests generated in one command"
echo -e "  ${GREEN}✔${RESET} Security scanning integrated into the developer workflow (stub-safe)"
echo -e "  ${GREEN}✔${RESET} GitOps-ready ArgoCD Application manifest, no backend required"
echo ""
echo -e "${DIM}All output was written to a temporary directory and will be cleaned up now.${RESET}"
echo ""

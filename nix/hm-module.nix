self:
{ config, lib, pkgs, ... }:

let
  cfg = config.programs.tmt;
  agentType = lib.types.enum [ "claude" "codex" ];
  yaml = pkgs.formats.yaml { };

  settings = lib.filterAttrs (_: v: v != null && v != { }) cfg.settings;

  stylixEnv =
    let
      colors = config.lib.stylix.colors;
      hex = colors.withHashtag;
      sgr = base: "38;2;${colors."${base}-rgb-r"};${colors."${base}-rgb-g"};${colors."${base}-rgb-b"}";
    in
    {
      TMT_FZF_COLORS = lib.concatStringsSep "," [
        "fg:${hex.base05}"
        "bg:-1"
        "hl:${hex.base09}"
        "fg+:${hex.base06}"
        "bg+:${hex.base01}"
        "hl+:${hex.base09}"
        "gutter:-1"
        "query:${hex.base05}"
        "info:${hex.base03}"
        "prompt:${hex.base0D}"
        "pointer:${hex.base09}"
        "marker:${hex.base0B}"
        "spinner:${hex.base0C}"
        "header:${hex.base03}"
        "border:${hex.base02}"
        "separator:${hex.base02}"
        "scrollbar:${hex.base02}"
        "label:${hex.base0A}"
        "preview-border:${hex.base02}"
        "preview-label:${hex.base04}"
      ];
      TMT_COLOR_WORKTREE = sgr "base0B";
      TMT_COLOR_MAIN = sgr "base03";
      TMT_COLOR_ROOT = sgr "base0D";
      TMT_COLOR_ACCENT = sgr "base0A";
      TMT_COLOR_DIM = sgr "base04";
    };
in
{
  options.programs.tmt = {
    enable = lib.mkEnableOption "tmt, a multi-repo worktree tool";

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.tmt;
      defaultText = lib.literalExpression "tmt.packages.\${pkgs.stdenv.hostPlatform.system}.tmt";
      description = "The tmt package to use.";
    };

    finalPackage = lib.mkOption {
      type = lib.types.package;
      readOnly = true;
      description = "The tmt package with tmuxPackage and environment applied.";
    };

    tmuxPackage = lib.mkOption {
      type = lib.types.package;
      default = config.programs.tmux.package;
      defaultText = lib.literalExpression "config.programs.tmux.package";
      description = "tmux bundled onto tmt's PATH.";
    };

    stylix.enable = lib.mkOption {
      type = lib.types.bool;
      default = (config.stylix.enable or false) && (config.stylix.autoEnable or true);
      defaultText = lib.literalExpression "config.stylix.enable && config.stylix.autoEnable";
      description = ''
        Theme tmt and its fzf pickers from stylix. Requires the stylix
        home-manager module; values set in environment take precedence.
      '';
    };

    environment = lib.mkOption {
      type = lib.types.attrsOf lib.types.str;
      default = { };
      example = lib.literalExpression ''
        {
          TMT_ROOT = "/home/me/tasks";
          TMT_BRANCH_PREFIX = "me/";
          TMT_COLOR_ACCENT = "38;5;221";
        }
      '';
      description = ''
        Default environment baked into the tmt binary. Each variable applies
        only when it is unset in the calling environment.
      '';
    };

    settings = {
      agent = lib.mkOption {
        type = lib.types.nullOr agentType;
        default = null;
        description = "Coding agent run in a new task's root window.";
      };

      agents = lib.mkOption {
        type = lib.types.attrsOf lib.types.str;
        default = { };
        example = { claude = "ccpm"; codex = "codex --dangerously-bypass-approvals-and-sandbox"; };
        description = "Command tmt runs for each agent.";
      };

      dirs = lib.mkOption {
        type = lib.types.attrsOf agentType;
        default = { };
        example = { "~/Documents/personal/website" = "codex"; };
        description = "Agent for tasks whose base is under the given directory (absolute or ~/-prefixed).";
      };
    };
  };

  config = lib.mkIf cfg.enable {
    programs.tmt.environment = lib.mkIf cfg.stylix.enable (lib.mapAttrs (_: lib.mkDefault) stylixEnv);

    programs.tmt.finalPackage = cfg.package.override {
      tmux = cfg.tmuxPackage;
      wrapperEnv = cfg.environment;
    };

    home.packages = [ cfg.finalPackage ];

    xdg.configFile."tmt/config.yaml" = lib.mkIf (settings != { }) {
      source = yaml.generate "tmt-config.yaml" settings;
    };
  };
}

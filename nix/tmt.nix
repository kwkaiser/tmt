{ lib
, buildGoModule
, writeText
, tmux
, git
, gh
, tea
, zoxide
, src
, version ? "dev"
, mainProgram ? "tmt"
, wrapperEnv ? { }
}:

let
  bundled = writeText "bundled_nix.go" ''
    package cli

    func init() {
    	bundledPath = ${builtins.toJSON (lib.makeBinPath [ tmux git gh tea zoxide ])}
    	bundledEnv = map[string]string{
    ${lib.concatStrings (lib.mapAttrsToList (name: value: "\t\t${builtins.toJSON name}: ${builtins.toJSON value},\n") wrapperEnv)}	}
    }
  '';

  depsKey = builtins.substring 0 16 (builtins.hashString "sha256"
    (builtins.readFile (src + "/go.mod") + builtins.readFile (src + "/go.sum")));
in
buildGoModule {
  pname = "tmt";
  inherit version src;

  vendorHash = "sha256-8zmuUVxsawWihml7vpKiOfcl2DEflmO2n7li1RQkQXI=";
  proxyVendor = true;
  overrideModAttrs = _: { name = "tmt-go-modules-${depsKey}"; };

  subPackages = [ "cmd/tmt" ];
  env.CGO_ENABLED = 0;

  ldflags = [
    "-s"
    "-w"
    "-X git.kwkaiser.io/kwkaiser/tmt/internal/cli.version=${version}"
  ];

  preBuild = ''
    cp ${bundled} internal/cli/bundled_nix.go
  '';

  postInstall = lib.optionalString (mainProgram != "tmt") ''
    mv $out/bin/tmt $out/bin/${mainProgram}
  '';

  doCheck = false;

  meta = { inherit mainProgram; };
}

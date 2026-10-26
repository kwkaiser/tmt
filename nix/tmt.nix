{ lib
, buildGoModule
, src
, version ? "dev"
}:

let
  depsKey = builtins.substring 0 16 (builtins.hashString "sha256"
    (builtins.readFile (src + "/go.mod") + builtins.readFile (src + "/go.sum")));
in
buildGoModule {
  pname = "tmt";
  inherit version src;

  vendorHash = "sha256-fddAzvfdUzVhR3APRVJP3FnNHJr/VAXGRNkbZoDucfo=";
  proxyVendor = true;
  overrideModAttrs = _: { name = "tmt-go-modules-${depsKey}"; };

  subPackages = [ "cmd/tmt" ];
  env.CGO_ENABLED = 0;

  ldflags = [
    "-s"
    "-w"
    "-X git.kwkaiser.io/kwkaiser/tmt/internal/cli.version=${version}"
  ];

  doCheck = false;

  meta.mainProgram = "tmt";
}

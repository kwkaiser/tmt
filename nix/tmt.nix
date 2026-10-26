{ lib
, buildGoModule
, makeWrapper
, tmux
, git
, fzf
, gh
, tea
, zoxide
, src
, version ? "dev"
, mainProgram ? "tmt"
, wrapperEnv ? { }
}:

let
  depsKey = builtins.substring 0 16 (builtins.hashString "sha256"
    (builtins.readFile (src + "/go.mod") + builtins.readFile (src + "/go.sum")));
in
buildGoModule {
  pname = "tmt";
  inherit version src;

  vendorHash = "sha256-w25D0I/zTmEIwO5MuzU6CFRDOlefxYwx/jMtwPS644Q=";
  proxyVendor = true;
  overrideModAttrs = _: { name = "tmt-go-modules-${depsKey}"; };

  subPackages = [ "cmd/tmt" ];
  env.CGO_ENABLED = 0;

  ldflags = [
    "-s"
    "-w"
    "-X git.kwkaiser.io/kwkaiser/tmt/internal/cli.version=${version}"
  ];

  nativeBuildInputs = [ makeWrapper ];

  postInstall = lib.optionalString (mainProgram != "tmt") ''
    mv $out/bin/tmt $out/bin/${mainProgram}
  '' + ''
    wrapProgram $out/bin/${mainProgram} \
      --prefix PATH : ${lib.makeBinPath [ tmux git fzf gh tea zoxide ]} \
      ${lib.concatStringsSep " " (lib.mapAttrsToList (name: value: "--set-default ${name} ${lib.escapeShellArg value}") wrapperEnv)}
  '';

  doCheck = false;

  meta = { inherit mainProgram; };
}

{
  description = "tmt cli";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};

        envOr = name: fallback:
          let value = builtins.getEnv name; in if value == "" then fallback else value;

        version = envOr "VERSION" (self.shortRev or self.dirtyShortRev or "dev");
      in
      {
        packages.tmt = pkgs.callPackage ./nix/tmt.nix {
          src = ./.;
          inherit version;
        };

        packages.default = self.packages.${system}.tmt;

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            go-task
            git
          ];
        };
      });
}

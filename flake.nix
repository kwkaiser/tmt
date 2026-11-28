{
  description = "tmt cli";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    let
      envOr = name: fallback:
        let value = builtins.getEnv name; in if value == "" then fallback else value;

      version = envOr "VERSION" (self.shortRev or self.dirtyShortRev or "dev");
    in
    flake-utils.lib.eachDefaultSystem
      (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
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
              gopls
              golangci-lint
              go-task
              git
            ];
          };
        })
    // {
      overlays.default = final: _: {
        tmt = final.callPackage ./nix/tmt.nix {
          src = self;
          inherit version;
        };
      };

      homeModules.default = import ./nix/hm-module.nix self;
      homeManagerModules.default = self.homeModules.default;
    };
}

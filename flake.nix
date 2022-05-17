{
  description = "The Screaming Image Flake";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs";
    flake-utils.url = "github:numtide/flake-utils";
    nixpkgs-fmt.url = "github:nix-community/nixpkgs-fmt";
  };

  outputs = { self, nixpkgs, flake-utils, nixpkgs-fmt }:
    flake-utils.lib.eachDefaultSystem (system:
      let pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        formatter = pkgs.nixpkgs-fmt;
        packages = {
          dis = pkgs.buildGoModule {
            name = "dis";
            src = ./.;
            vendorSha256 = "sha256-+bH4meSDxc8Fznf/ZuPQ7FNqtqTMLmAupmfPEOlCY1w=";
          };
        };

        devShell = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
          ];
        };
      });
}

{
  description = "The Screaming Image Flake";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let pkgs = nixpkgs.legacyPackages.${system};
      in {
        packages.hello = pkgs.hello;
	packages.dis = pkgs.buildGoModule {
	  name = "dis";
	  src = ./.;
          vendorSha256 = "sha256-+bH4meSDxc8Fznf/ZuPQ7FNqtqTMLmAupmfPEOlCY1w=";
	};

        devShell = pkgs.mkShell { buildInputs = with pkgs; [
  	  go
	  gopls
        ];};
      });
}

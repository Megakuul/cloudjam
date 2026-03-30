{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/25.11";
  };

  outputs =
    { self, ... }@inputs:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
    in
    {
      devShells = inputs.nixpkgs.lib.genAttrs systems (
        system:
        let
          pkgs = import inputs.nixpkgs { inherit system; };
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              nodejs-slim_24
              corepack
              go
              awscli2
              localstack
              podman
              podman-compose
            ];
          };
        }
      );
      nixosModules.hornet =
        { lib, ... }:
        {
          options.cloudjam = {
            addr = lib.mkOption {
              type = lib.types.str;
              default = "0.0.0.0:9000";
              description = "Host address of the hornet engine";
            };
          };
          imports = [
            ./nix/hornet
          ];
        };
    };
}

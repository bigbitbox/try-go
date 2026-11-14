{
  description = "try-go: a terminal workspace manager";

  inputs.nixpkgs.url = "nixpkgs";

  outputs = { nixpkgs, ... }:
    let
      systems = [ "aarch64-darwin" "x86_64-darwin" "aarch64-linux" "x86_64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
      pkgsFor = system: import nixpkgs { inherit system; };
      packageFor = system:
        let pkgs = pkgsFor system;
        in pkgs.buildGoModule {
          pname = "try-go";
          version = "0.1.0";
          src = pkgs.lib.fileset.toSource {
            root = ./.;
            fileset = pkgs.lib.fileset.unions [ ./go.mod ./go.sum ./cmd ./internal ];
          };
          vendorHash = "sha256-JGQ/UHaGj8t8G/stfcTTnGtifw8ZfbxCzByzH5METyo=";
          subPackages = [ "cmd/try-go" ];
          env.CGO_ENABLED = 0;
          ldflags = [ "-s" "-w" ];
          nativeCheckInputs = [ pkgs.git ];
          checkPhase = ''
            runHook preCheck
            go test ./...
            runHook postCheck
          '';
          meta = {
            description = "Create, find, and manage development workspaces";
            mainProgram = "try-go";
            platforms = systems;
          };
        };
    in {
      packages = forAllSystems (system: rec {
        try-go = packageFor system;
        default = try-go;
      });
      devShells = forAllSystems (system:
        let pkgs = pkgsFor system;
        in {
          default = pkgs.mkShell {
            packages = [ pkgs.go pkgs.gopls pkgs.just pkgs.git pkgs.python3 ];
            CGO_ENABLED = "0";
          };
        });
      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = "${packageFor system}/bin/try-go";
        };
      });
    };
}

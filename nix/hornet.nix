{ pkgs, ... }:
{
  virtualization.oci-containers.containers = {
    cloudjam-database = {
      image = "ghcr.io/documentdb/documentdb/documentdb-oss:latest";
      ports = [ "127.0.0.1:10260:10260" ];
      volumes = [

      ];
    };
  };
}

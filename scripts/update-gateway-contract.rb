#!/usr/bin/env ruby
# frozen_string_literal: true

require "digest"
require "json"
require "open3"
require "uri"

abort("usage: #{$PROGRAM_NAME} GATEWAY_REPO EXACT_40_HEX_REVISION [OUTPUT_MANIFEST PRODUCER_URL]") unless [2, 4].include?(ARGV.length)

gateway_repo, revision, output_manifest, producer_url = ARGV
if producer_url
  begin
    producer = URI.parse(producer_url)
  rescue URI::InvalidURIError
    abort("producer URL must be a credential-free HTTPS URL")
  end
  abort("producer URL must be a credential-free HTTPS URL") unless producer.scheme == "https" && producer.host && !producer.userinfo && !producer.fragment
end
abort("revision must be exactly 40 lowercase hexadecimal characters") unless revision.match?(/\A[0-9a-f]{40}\z/)

def git_bytes(repo, *args)
  stdout, stderr, status = Open3.capture3("git", "-C", repo, *args)
  abort("git #{args.join(' ')} failed: #{stderr}") unless status.success?
  stdout
end

resolved = git_bytes(gateway_repo, "rev-parse", "#{revision}^{commit}").strip
abort("selected revision resolved to #{resolved}, not #{revision}") unless resolved == revision

default_path = File.expand_path("../testdata/contracts/gateway-api.json", __dir__)
manifest_path = output_manifest ? File.expand_path(output_manifest) : default_path
manifest = JSON.parse(File.binread(default_path))
source = manifest.fetch("source")
source["repository"] = producer_url || "https://github.com/loxilb-io/loxilb-inference-gateway"
source["revision"] = revision
source["swagger_sha256"] = Digest::SHA256.hexdigest(git_bytes(gateway_repo, "show", "#{revision}:api/swagger.yml"))
source["swagger_extras_sha256"] = Digest::SHA256.hexdigest(git_bytes(gateway_repo, "show", "#{revision}:api/swagger-extras.yml"))

File.binwrite(manifest_path, JSON.pretty_generate(manifest) + "\n")
warn("updated #{manifest_path} from exact Gateway revision #{revision}")

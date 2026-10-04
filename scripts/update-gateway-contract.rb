#!/usr/bin/env ruby
# frozen_string_literal: true
require "digest"
require "json"
require "open3"
require "optparse"
require "uri"
require "fileutils"
model = "general"
OptionParser.new { |opts| opts.on("--model MODEL") { |value| model = value } }.parse!
origins = {"general" => "https://github.com/loxilb-io/loxilb-inference-gateway", "kcmvp" => "https://github.com/netlox-io/loxilb-igw"}.freeze
abort "unknown model" unless origins.key?(model)
abort "usage: #{$PROGRAM_NAME} [--model general|kcmvp] REPO EXACT_REVISION [OUTPUT_MANIFEST PRODUCER_URL]" unless [2,4].include?(ARGV.length)
repo, revision, output_manifest, producer_url = ARGV
abort "revision must be exactly 40 lowercase hexadecimal characters" unless revision.match?(/\A[0-9a-f]{40}\z/)
if producer_url
  begin
    producer = URI.parse(producer_url)
  rescue URI::InvalidURIError
    abort "producer URL must be a credential-free HTTPS URL"
  end
  abort "producer URL must be a credential-free HTTPS URL" unless producer.scheme == "https" && producer.host && !producer.userinfo && !producer.fragment
end
def git_bytes(repo, *args)
  stdout, stderr, status = Open3.capture3({"PATH" => "/usr/bin:/bin", "HOME" => "/nonexistent", "GIT_CONFIG_NOSYSTEM" => "1", "GIT_CONFIG_GLOBAL" => "/dev/null", "GIT_CONFIG_SYSTEM" => "/dev/null"}, "/usr/bin/git", "--no-replace-objects", "-C", repo, *args, unsetenv_others: true)
  abort "Git producer read refused: #{stderr}" unless status.success?
  stdout
end
origin = git_bytes(repo, "remote", "get-url", "origin").strip.sub(/\.git\z/, "")
abort "wrong-model Gateway origin" unless origin == (producer_url || origins.fetch(model))
abort "revision did not resolve exactly" unless git_bytes(repo, "rev-parse", "#{revision}^{commit}").strip == revision
base = File.expand_path("../testdata/contracts", __dir__)
default_path = model == "general" ? File.join(base, "gateway-api.json") : File.join(base, "models", model, "gateway-api.json")
path = output_manifest ? File.expand_path(output_manifest) : default_path
manifest = JSON.parse(File.binread(default_path))
manifest["model"] = model
source = manifest.fetch("source")
source["repository"] = origin
source["revision"] = revision
source["swagger_sha256"] = Digest::SHA256.hexdigest(git_bytes(repo, "show", "#{revision}:api/swagger.yml"))
source["swagger_extras_sha256"] = Digest::SHA256.hexdigest(git_bytes(repo, "show", "#{revision}:api/swagger-extras.yml"))
FileUtils.mkdir_p(File.dirname(path))
File.binwrite(path, JSON.pretty_generate(manifest) + "\n")
warn "updated #{path} from immutable #{model} Gateway blobs #{revision}"

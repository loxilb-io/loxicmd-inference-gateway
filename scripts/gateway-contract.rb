#!/usr/bin/env ruby
# frozen_string_literal: true
require "json"
model = ARGV.shift || "general"
abort "usage: gateway-contract.rb [general|kcmvp]" unless ARGV.empty? && %w[general kcmvp].include?(model)
base = File.expand_path("../testdata/contracts", __dir__)
path = model == "general" ? File.join(base, "gateway-api.json") : File.join(base, "models", model, "gateway-api.json")
manifest = JSON.parse(File.binread(path))
origin = model == "general" ? "https://github.com/loxilb-io/loxilb-inference-gateway" : "https://github.com/netlox-io/loxilb-igw"
abort "wrong-model contract" unless manifest["model"] == model && manifest.dig("source", "repository") == origin
abort "unpinned contract" unless manifest.dig("source", "revision").to_s.match?(/\A[0-9a-f]{40}\z/)
digest = manifest.dig("source", "swagger_sha256")
abort "missing contract digest" unless digest.to_s.match?(/\A[0-9a-f]{64}\z/)
abort "missing extras contract digest" unless manifest.dig("source", "swagger_extras_sha256").to_s.match?(/\A[0-9a-f]{64}\z/)
puts digest

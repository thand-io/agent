# Documentation Development

## Local Setup

### Prerequisites

1. **Install rbenv** (if not already installed):
   ```bash
   brew install rbenv
   ```

2. **Initialize rbenv** in your shell:
   ```bash
   echo 'eval "$(/opt/homebrew/bin/rbenv init -)"' >> ~/.zshrc
   source ~/.zshrc
   ```

3. **Install Ruby 3.2.0** (if not already installed):
   ```bash
   rbenv install 3.2.0
   ```

### Running Jekyll Locally

1. **Navigate to docs directory**:
   ```bash
   cd docs
   ```

2. **Run the setup script** (first time only):
   ```bash
   ./setup.sh
   ```

3. **Start the development server**:
   ```bash
   bundle exec jekyll serve
   ```

4. **View your site** at: http://localhost:4000/

### Platform Support

The Gemfile.lock is **not committed** to the repository because:
- Different platforms (macOS, Linux, Windows) have different native gem requirements
- GitHub Actions resolves dependencies fresh each time for consistency
- Avoids bundler "frozen mode" conflicts between developers

The setup supports:
- **macOS** (arm64-darwin, x86_64-darwin)
- **Linux** (x86_64-linux) - for GitHub Actions
- **Windows** - for Windows developers

### Common Commands

```bash
# Build the site
bundle exec jekyll build

# Serve with live reload
bundle exec jekyll serve --livereload

# Serve on all interfaces (for testing on other devices)
bundle exec jekyll serve --host 0.0.0.0

# Clean generated files
bundle exec jekyll clean
```

### Troubleshooting

#### Ruby Version Issues
If you see errors related to Ruby versions or gems:

```bash
# Ensure correct Ruby version
rbenv local 3.2.0
ruby --version  # Should show 3.2.0

# Remove lock file and reinstall gems
rm -f Gemfile.lock
bundle install
```

#### Bundler Frozen Mode
If you see "frozen mode" errors:

```bash
# Unset frozen mode and reinstall
bundle config unset frozen
rm -f Gemfile.lock
bundle install
```

#### Protobuf Errors
If you see `google/protobuf_c` errors, the setup script should fix this by:
- Using Ruby 3.2.0 instead of system Ruby
- Pinning `google-protobuf` to a compatible version

#### Jekyll Won't Start
```bash
# Check if another process is using port 4000
lsof -i :4000

# Try a different port
bundle exec jekyll serve --port 4001
```

### Documentation Structure

```
docs/
├── _config.yml          # Jekyll configuration
├── Gemfile             # Ruby dependencies
├── index.md            # Homepage
├── getting-started.md  # Quick start guide
├── api.md             # API documentation
├── setup/             # Setup guides
│   ├── index.md       # Setup overview
│   ├── gcp.md         # Google Cloud setup
│   └── local.md       # Local development
└── configuration/     # Configuration docs
    └── index.md       # Config reference
```

### Adding New Pages

1. Create a new `.md` file
2. Add YAML frontmatter:
   ```yaml
   ---
   layout: default
   title: Page Title
   nav_order: 5
   description: "Page description"
   ---
   ```
3. Write content in Markdown
4. Jekyll will automatically build and serve it

### Theme Documentation

Using the `just-the-docs` theme. See:
- [Theme Documentation](https://just-the-docs.github.io/just-the-docs/)
- [Configuration Options](https://just-the-docs.github.io/just-the-docs/docs/configuration/)

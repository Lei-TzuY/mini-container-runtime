package builder

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"minicontainer/internal/imagestore"
	"minicontainer/internal/state"
)

// BuildOptions options for minictl build.
type BuildOptions struct {
	ContextDir string
	Dockerfile string
	Tag        string
	OutputDir  string
	Store      *state.Store
}

// BuildResult holds outcome metadata of Dockerfile build.
type BuildResult struct {
	Image *state.Image
	Logs  []string
}

// BuildDockerfile parses and executes Dockerfile directives to create a container rootfs image.
func BuildDockerfile(opts BuildOptions) (*BuildResult, error) {
	if opts.ContextDir == "" {
		return nil, fmt.Errorf("context directory is required")
	}
	dockerfilePath := opts.Dockerfile
	if dockerfilePath == "" {
		dockerfilePath = filepath.Join(opts.ContextDir, "Dockerfile")
	}

	file, err := os.Open(dockerfilePath)
	if err != nil {
		return nil, fmt.Errorf("open Dockerfile %q: %w", dockerfilePath, err)
	}
	defer file.Close()

	imgID := imagestore.GenerateImageID()
	if opts.OutputDir == "" {
		home, _ := os.UserHomeDir()
		if home == "" {
			home = "/tmp"
		}
		cleanTag := strings.NewReplacer(":", "_", "/", "_").Replace(opts.Tag)
		if cleanTag == "" {
			cleanTag = imgID
		}
		opts.OutputDir = filepath.Join(home, ".minicontainer", "builds", cleanTag)
	}

	if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("create build output dir: %w", err)
	}
	if _, err := canonicalBuildRoot(opts.OutputDir); err != nil {
		return nil, fmt.Errorf("validate build output dir: %w", err)
	}

	repo, tag := imagestore.ParseRepositoryTag(opts.Tag)
	img := &state.Image{
		ID:         imgID,
		Repository: repo,
		Tag:        tag,
		Name:       opts.Tag,
		RootFS:     opts.OutputDir,
		LoadedAt:   time.Now(),
		WorkDir:    "/",
	}

	var logs []string
	log := func(msg string) {
		logs = append(logs, msg)
		fmt.Println(msg)
	}

	log(fmt.Sprintf("Building image %s (ID: %s)...", opts.Tag, imgID))

	scanner := bufio.NewScanner(file)
	workDir := "/"

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		cmd := strings.ToUpper(parts[0])
		args := strings.TrimSpace(line[len(parts[0]):])

		switch cmd {
		case "FROM":
			log(fmt.Sprintf("Step: FROM %s", args))
			if opts.Store != nil {
				if baseImg, err := opts.Store.GetImage(args); err == nil && baseImg.RootFS != "" {
					if err := copyTree(baseImg.RootFS, opts.OutputDir, "/", true); err != nil {
						return nil, fmt.Errorf("copy base image rootfs: %w", err)
					}
					log(fmt.Sprintf("  Loaded base rootfs from image %s", args))
					continue
				}
			}
			// If base image is directory path.
			if info, err := os.Stat(args); err == nil && info.IsDir() {
				if err := copyTree(args, opts.OutputDir, "/", true); err != nil {
					return nil, fmt.Errorf("copy base directory %s: %w", args, err)
				}
				log(fmt.Sprintf("  Loaded base rootfs from directory %s", args))
			} else {
				log(fmt.Sprintf("  Warning: Base image/directory %q not found locally. Initializing empty rootfs base.", args))
			}

		case "WORKDIR":
			log(fmt.Sprintf("Step: WORKDIR %s", args))
			logical, err := normalizeContainerPath(workDir, args)
			if err != nil {
				return nil, fmt.Errorf("WORKDIR %q: %w", args, err)
			}
			workDir = logical
			img.WorkDir = logical
			if err := mkdirRootFSPath(opts.OutputDir, logical, 0755); err != nil {
				return nil, fmt.Errorf("create WORKDIR %q: %w", logical, err)
			}

		case "ENV":
			log(fmt.Sprintf("Step: ENV %s", args))
			img.Env = append(img.Env, args)

		case "EXPOSE":
			log(fmt.Sprintf("Step: EXPOSE %s", args))
			img.ExposedPorts = append(img.ExposedPorts, args)

		case "CMD":
			log(fmt.Sprintf("Step: CMD %s", args))
			img.Cmd = parseArrayOrString(args)

		case "ENTRYPOINT":
			log(fmt.Sprintf("Step: ENTRYPOINT %s", args))
			img.Cmd = parseArrayOrString(args)

		case "COPY":
			log(fmt.Sprintf("Step: COPY %s", args))
			copyParts := strings.Fields(args)
			if len(copyParts) < 2 {
				return nil, fmt.Errorf("COPY requires source and destination args")
			}
			src := copyParts[0]
			dst := copyParts[1]

			srcPath, err := resolveBuildContextSource(opts.ContextDir, src)
			if err != nil {
				return nil, err
			}
			srcInfo, err := os.Lstat(srcPath)
			if err != nil {
				return nil, fmt.Errorf("inspect COPY source %q: %w", src, err)
			}
			dstLogical, err := normalizeContainerPath(workDir, dst)
			if err != nil {
				return nil, fmt.Errorf("COPY destination %q: %w", dst, err)
			}
			dstIsDir, err := destinationIsDirectory(opts.OutputDir, dstLogical)
			if err != nil {
				return nil, fmt.Errorf("inspect COPY destination %q: %w", dst, err)
			}
			if dstIsDir || dst == "." || strings.HasSuffix(dst, "/") || strings.HasSuffix(dst, "\\") {
				if err := mkdirRootFSPath(opts.OutputDir, dstLogical, 0755); err != nil {
					return nil, fmt.Errorf("create COPY destination %q: %w", dst, err)
				}
				dstLogical = path.Join(dstLogical, filepath.Base(srcPath))
			}

			if srcInfo.IsDir() {
				if err := copyTree(srcPath, opts.OutputDir, dstLogical, false); err != nil {
					return nil, fmt.Errorf("COPY dir %s to %s failed: %w", src, dst, err)
				}
			} else {
				if err := copyRegularFile(srcPath, opts.OutputDir, dstLogical, srcInfo.Mode()); err != nil {
					return nil, fmt.Errorf("COPY file %s to %s failed: %w", src, dst, err)
				}
			}
			log(fmt.Sprintf("  Copied %s -> %s", src, dst))

		case "RUN":
			log(fmt.Sprintf("Step: RUN %s", args))
			// Execute simple shell inline script if file creation / echo.
			if strings.HasPrefix(args, "echo ") && strings.Contains(args, ">") {
				echoParts := strings.SplitN(args, ">", 2)
				val := strings.TrimSpace(strings.TrimPrefix(echoParts[0], "echo"))
				val = strings.Trim(val, "\"'")
				outFile := strings.TrimSpace(echoParts[1])
				targetLogical, err := normalizeContainerPath(workDir, outFile)
				if err != nil {
					return nil, fmt.Errorf("RUN output %q: %w", outFile, err)
				}
				if err := mkdirRootFSPath(opts.OutputDir, path.Dir(targetLogical), 0755); err != nil {
					return nil, fmt.Errorf("create RUN output parent: %w", err)
				}
				targetFile, err := resolveRootFSPath(opts.OutputDir, targetLogical)
				if err != nil {
					return nil, fmt.Errorf("resolve RUN output: %w", err)
				}
				if err := os.WriteFile(targetFile, []byte(val+"\n"), 0644); err != nil {
					return nil, fmt.Errorf("write RUN output: %w", err)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan Dockerfile: %w", err)
	}

	sz, _ := imagestore.CalculateDirSize(opts.OutputDir)
	img.Size = sz

	if opts.Store != nil {
		if err := opts.Store.PublishImage(img); err != nil {
			return nil, fmt.Errorf("save image state: %w", err)
		}
	}

	log(fmt.Sprintf("Successfully built image %s (Size: %d bytes)", opts.Tag, sz))
	return &BuildResult{Image: img, Logs: logs}, nil
}

func parseArrayOrString(val string) []string {
	val = strings.TrimSpace(val)
	if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
		content := val[1 : len(val)-1]
		rawParts := strings.Split(content, ",")
		var out []string
		for _, p := range rawParts {
			cleaned := strings.TrimSpace(p)
			cleaned = strings.Trim(cleaned, "\"'")
			if cleaned != "" {
				out = append(out, cleaned)
			}
		}
		return out
	}
	return strings.Fields(val)
}

package commands

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gosimple/slug"
	"github.com/jackchuka/confluence-md/internal/confluence"
	confluenceModel "github.com/jackchuka/confluence-md/internal/confluence/model"
	"github.com/jackchuka/confluence-md/internal/converter"
)

// sanitizeFileName uses the mature gosimple/slug library for robust filename sanitization
func sanitizeFileName(name string) string {
	if name == "" {
		return "untitled"
	}

	sanitized := slug.MakeLang(name, "en")

	if sanitized == "" {
		return name
	}

	return sanitized
}

func buildOutputNamer(template string) (converter.OutputNamer, error) {
	if strings.TrimSpace(template) == "" {
		return nil, nil
	}

	namer, err := converter.NewTemplateOutputNamer(template)
	if err != nil {
		return nil, err
	}

	return namer, nil
}

// PageConversionResult represents the result of converting a single page
type PageConversionResult struct {
	OutputPath  string
	PageID      string
	Title       string
	ImagesCount int
	Success     bool
	Error       error
}

// convertSinglePage handles the full conversion pipeline for a single page
func convertSinglePage(client confluence.Client, page *confluenceModel.ConfluencePage, baseURL string, opts PageOptions) *PageConversionResult {
	return convertSinglePageWithPath(client, page, baseURL, "", opts)
}

// convertSinglePageWithPath handles conversion with a custom output path (for tree structure)
func convertSinglePageWithPath(client confluence.Client, page *confluenceModel.ConfluencePage, baseURL, outputPath string, opts PageOptions) *PageConversionResult {
	result := &PageConversionResult{
		PageID: page.ID,
		Title:  page.Title,
	}

	if outputPath == "" {
		fileName, err := converter.GenerateFileName(page, opts.OutputNamer)
		if err != nil {
			result.Error = fmt.Errorf("failed to generate output filename: %w", err)
			return result
		}
		outputPath = filepath.Join(opts.OutputDir, fileName)
	}
	result.OutputPath = outputPath

	// Create converter and convert page
	var options []converter.Option
	if opts.DownloadImages {
		options = append(options, converter.WithDownloadAttachments(opts.ImageFolder))
	}
	conv := converter.NewConverter(client, options...)
	doc, err := conv.ConvertPage(page, baseURL, filepath.Dir(outputPath))
	if err != nil {
		result.Error = fmt.Errorf("failed to convert page: %w", err)
		return result
	}
	result.ImagesCount = len(doc.Images)

	if err := converter.SaveMarkdownDocument(doc, outputPath, opts.IncludeMetadata); err != nil {
		result.Error = fmt.Errorf("failed to save document: %w", err)
		return result
	}

	result.Success = true
	return result
}

// printConversionResult prints the result of a page conversion in a consistent format
func printConversionResult(result *PageConversionResult) {
	if result.Success {
		fmt.Printf("✅ Successfully converted page: %s\n", result.OutputPath)
		fmt.Printf("   Page ID: %s\n", result.PageID)
		fmt.Printf("   Title: %s\n", result.Title)
		if result.ImagesCount > 0 {
			fmt.Printf("   📥 Images downloaded: %d\n", result.ImagesCount)
		}
	} else {
		fmt.Printf("❌ Failed to convert page: %s\n", result.Title)
		if result.Error != nil {
			fmt.Printf("   Error: %v\n", result.Error)
		}
	}
	fmt.Println()
}

func urlToPageInfo(pageURL string) (confluenceModel.PageURLInfo, error) {
	if pageURL == "" {
		return confluenceModel.PageURLInfo{}, fmt.Errorf("URL is empty")
	}

	u, err := url.Parse(pageURL)
	if err != nil {
		return confluenceModel.PageURLInfo{}, fmt.Errorf("invalid URL: %w", err)
	}

	baseURL := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	var pageID string
	var spaceKey string
	var title string

	// Extract page ID from path
	// Path format: 
	// /display/SPACE/Title
	// /pages/viewpage.action?pageId=622848016
	if strings.HasPrefix(u.Path, "/display/") {
		// 去除前缀后按 "/" 分割
		// TrimPrefix 变成 "SPACE/Title"
		// SplitN 限制分割次数，防止 Title 中包含 "/" 导致被截断（尽管 Title 通常不含 /）
		parts := strings.SplitN(strings.TrimPrefix(u.Path, "/display/"), "/", 2)

		if len(parts) == 2 {
			spaceKey = parts[0]
			title = parts[1] 
			// 注意：u.Path 已经被自动解码了（例如 %20 会变成空格），所以这里不需要额外解码
		} else {
			return confluenceModel.PageURLInfo{}, fmt.Errorf("could not extract page space and title from URL")
		}

		// 情况 2: /pages/viewpage.action?pageId=...
	} else if strings.Contains(u.Path, "viewpage.action") {
		// 获取查询参数 (query params)
		queryParams := u.Query()
		if pageID = queryParams.Get("pageId"); pageID == "" {
			return confluenceModel.PageURLInfo{}, fmt.Errorf("could not extract page id from URL")
		}
	} else {
		return confluenceModel.PageURLInfo{}, fmt.Errorf("Invalid URL")
	}

	return confluenceModel.PageURLInfo{
		BaseURL:  baseURL,
		PageID:   pageID,
		SpaceKey: spaceKey,
		Title:    title,
	}, nil
}

$API = "http://localhost:8080/api/tools/register"
$HEADERS = @{ "Content-Type" = "application/json" }
$BIAOSHU_TOOLS_BASE = $env:BIAOSHU_TOOLS_BASE
if (-not $BIAOSHU_TOOLS_BASE) {
    $BIAOSHU_TOOLS_BASE = "http://127.0.0.1:9001"
}

function Register-BiaoshuTool {
    param(
        [Parameter(Mandatory = $true)][hashtable]$Manifest
    )

    Write-Host ("  registering {0}..." -f $Manifest.name) -NoNewline
    $body = $Manifest | ConvertTo-Json -Depth 10
    try {
        Invoke-RestMethod -Uri $API -Method Post -Headers $HEADERS -Body $body | Out-Null
        Write-Host " ok" -ForegroundColor Green
    } catch {
        Write-Host (" failed: {0}" -f $_.Exception.Message) -ForegroundColor Red
    }
}

Write-Host "Registering biaoshu-writer tools to cloud-backend..." -ForegroundColor Cyan

Register-BiaoshuTool @{
    name = "parse_bid_files"
    description = "Parse tender/bid files (txt/pdf/docx/xlsx) and extract scoring criteria, procurement requirements, and technical requirements."
    version = "1.0.0"
    type = "http"
    capabilities = @("bid_writing", "bid_parsing", "document_parsing")
    tags = @("bid_writing", "bid", "tender", "biaoshu")
    endpoint = "$BIAOSHU_TOOLS_BASE/tools/parse_bid_files"
    timeout = 300
    parameters = @{
        file_path = @{ type = "string"; description = "Absolute path of the tender file"; required = $true }
        output_dir = @{ type = "string"; description = "Optional output subdirectory name"; required = $false }
    }
    output = @{
        stdout = @{ type = "string"; description = "Full parsing report" }
        raw_text_path = @{ type = "string"; description = "Parsed raw text markdown file path" }
        report_path = @{ type = "string"; description = "Generated markdown report path" }
    }
}

Register-BiaoshuTool @{
    name = "check_chapter_words"
    description = "Check one technical-bid chapter word count from markdown content or a markdown file."
    version = "1.0.0"
    type = "http"
    capabilities = @("bid_quality_check")
    tags = @("bid", "biaoshu", "word_count")
    endpoint = "$BIAOSHU_TOOLS_BASE/tools/check_chapter_words"
    timeout = 30
    parameters = @{
        content = @{ type = "string"; description = "Markdown chapter content"; required = $false }
        file_path = @{ type = "string"; description = "Markdown chapter file path"; required = $false }
    }
    output = @{
        stdout = @{ type = "string"; description = "Word-count report" }
    }
}

Register-BiaoshuTool @{
    name = "check_all_chapters_words"
    description = "Check word counts for all markdown chapters in a chapter directory."
    version = "1.0.0"
    type = "http"
    capabilities = @("bid_quality_check")
    tags = @("bid", "biaoshu", "word_count")
    endpoint = "$BIAOSHU_TOOLS_BASE/tools/check_all_chapters_words"
    timeout = 60
    parameters = @{
        chapter_dir = @{ type = "string"; description = "Chapter directory path"; required = $true }
        score_map = @{ type = "object"; description = "Optional chapter score map"; required = $false }
        total_pages = @{ type = "number"; description = "Target total pages"; required = $false }
        total_score = @{ type = "number"; description = "Total score"; required = $false }
    }
    output = @{
        overall_pass = @{ type = "boolean"; description = "Whether all chapters passed" }
        chapters = @{ type = "array"; description = "Per-chapter results" }
    }
}

Register-BiaoshuTool @{
    name = "convert_to_word"
    description = "Convert a markdown technical bid to a Word document."
    version = "1.0.0"
    type = "http"
    capabilities = @("document_export")
    tags = @("bid", "biaoshu", "word", "docx")
    endpoint = "$BIAOSHU_TOOLS_BASE/tools/convert_to_word"
    timeout = 60
    parameters = @{
        content = @{ type = "string"; description = "Markdown content"; required = $false }
        input_file = @{ type = "string"; description = "Markdown input file"; required = $false }
        output_file = @{ type = "string"; description = "Word output file"; required = $false }
        project_name = @{ type = "string"; description = "Project name for generated filename"; required = $false }
        format_profile = @{ type = "string"; description = "Format profile: government/highway/waterway"; required = $false }
    }
    output = @{
        output_path = @{ type = "string"; description = "Generated Word path" }
        output_name = @{ type = "string"; description = "Generated Word filename" }
    }
}

Register-BiaoshuTool @{
    name = "merge_chapters"
    description = "Merge markdown chapter files into one markdown document."
    version = "1.0.0"
    type = "http"
    capabilities = @("document_assembly")
    tags = @("bid", "biaoshu", "markdown")
    endpoint = "$BIAOSHU_TOOLS_BASE/tools/merge_chapters"
    timeout = 30
    parameters = @{
        chapter_dir = @{ type = "string"; description = "Chapter directory path"; required = $true }
        output_file = @{ type = "string"; description = "Merged markdown output file"; required = $false }
    }
    output = @{
        output_path = @{ type = "string"; description = "Merged markdown path" }
    }
}

Register-BiaoshuTool @{
    name = "rag_retrieve"
    description = "Retrieve reference chunks from the biaoshu Chroma vector database."
    version = "1.0.0"
    type = "http"
    capabilities = @("bid_reference_retrieval")
    tags = @("bid", "biaoshu", "rag", "retrieval")
    endpoint = "$BIAOSHU_TOOLS_BASE/tools/rag_retrieve"
    timeout = 60
    parameters = @{
        query = @{ type = "string"; description = "Retrieval query"; required = $true }
        k = @{ type = "number"; description = "Number of results"; required = $false }
        config = @{ type = "string"; description = "Embedding config path"; required = $false }
        persist_dir = @{ type = "string"; description = "Chroma persist directory"; required = $false }
        collection = @{ type = "string"; description = "Chroma collection name"; required = $false }
    }
    output = @{
        stdout = @{ type = "string"; description = "Retrieval report" }
    }
}

Register-BiaoshuTool @{
    name = "chapter_chunker"
    description = "Split a markdown technical bid into JSONL chunks for RAG indexing."
    version = "1.0.0"
    type = "http"
    capabilities = @("bid_reference_indexing")
    tags = @("bid", "biaoshu", "rag", "chunking")
    endpoint = "$BIAOSHU_TOOLS_BASE/tools/chapter_chunker"
    timeout = 60
    parameters = @{
        input_file = @{ type = "string"; description = "Markdown input file"; required = $true }
        output_file = @{ type = "string"; description = "JSONL output file"; required = $false }
        chunk_size = @{ type = "number"; description = "Chunk size in characters"; required = $false }
        chunk_overlap = @{ type = "number"; description = "Chunk overlap in characters"; required = $false }
    }
    output = @{
        output_path = @{ type = "string"; description = "Generated JSONL path" }
    }
}

Register-BiaoshuTool @{
    name = "embed_chunks"
    description = "Write JSONL chunks into a Chroma vector database."
    version = "1.0.0"
    type = "http"
    capabilities = @("bid_reference_indexing")
    tags = @("bid", "biaoshu", "rag", "embedding")
    endpoint = "$BIAOSHU_TOOLS_BASE/tools/embed_chunks"
    timeout = 120
    parameters = @{
        input_file = @{ type = "string"; description = "Chunk JSONL input file"; required = $true }
        config = @{ type = "string"; description = "Embedding config path"; required = $false }
        persist_dir = @{ type = "string"; description = "Chroma persist directory"; required = $false }
        collection = @{ type = "string"; description = "Chroma collection name"; required = $false }
    }
    output = @{
        stdout = @{ type = "string"; description = "Indexing report" }
    }
}

Write-Host ""
Write-Host "Done. Verify with: curl http://localhost:9090/api/tools/parse_bid_files" -ForegroundColor Yellow

package domain

type FileType string

const (
	FileMultiband  FileType = "multiband"
	FileNatural    FileType = "natural"
	FileFalseColor FileType = "false_color"
	FileNDVI       FileType = "ndvi"
	FileNDRE       FileType = "ndre"
	FileEVI        FileType = "evi"
	FileGNDVI      FileType = "gndvi"
	FileNBR        FileType = "nbr"
	FileNDMI       FileType = "ndmi"
	FileSAVI       FileType = "savi"
	FileRedEdge    FileType = "red_edge"
	FileSWIR       FileType = "swir"
	FileParams     FileType = "params"
	FileAnalisis   FileType = "analisis"
	FileIAReq      FileType = "ia_req"
	FileIAResult   FileType = "ia"
)

func AllImageTypes() []FileType {
	return []FileType{
		FileNatural, FileFalseColor, FileNDVI, FileNDRE,
		FileEVI, FileGNDVI, FileNBR, FileNDMI,
		FileSAVI, FileRedEdge, FileSWIR,
	}
}

import React, { useState, useEffect, useRef } from 'react';

function App() {
    const [activeTab, setActiveTab] = useState('disk');
    const [files, setFiles] = useState([]);
    const [stats, setStats] = useState(null);
    const [uploadPath, setUploadPath] = useState('');
    const [isRefreshing, setIsRefreshing] = useState(false);
    const [uploadProgress, setUploadProgress] = useState({});
    const [customPaths, setCustomPaths] = useState([]);
    const [newCustomPath, setNewCustomPath] = useState('');
    const [previewUrl, setPreviewUrl] = useState(null);
    const [selectedFile, setSelectedFile] = useState(null);
    const [conversionType, setConversionType] = useState('none');
    const [showExplorer, setShowExplorer] = useState(false);
    const [explorerPath, setExplorerPath] = useState('/');
    const [explorerItems, setExplorerItems] = useState([]);
    const [explorerLoading, setExplorerLoading] = useState(false);
    const fileInputRef = useRef(null);

    useEffect(() => {
        loadFiles();
        loadStats();
        loadCustomPaths();

        const refreshInterval = setInterval(() => {
            loadFiles();
            loadStats();
        }, 5000);

        return () => clearInterval(refreshInterval);
    }, [activeTab]);

    const loadCustomPaths = () => {
        const paths = localStorage.getItem('customPaths');
        if (paths) {
            setCustomPaths(JSON.parse(paths));
        }
    };

    const addCustomPath = (e) => {
        e.preventDefault();
        if (!newCustomPath) return;
        
        const updatedPaths = [...customPaths, newCustomPath];
        setCustomPaths(updatedPaths);
        localStorage.setItem('customPaths', JSON.stringify(updatedPaths));
        setNewCustomPath('');
    };

    const removeCustomPath = (path) => {
        const updatedPaths = customPaths.filter(p => p !== path);
        setCustomPaths(updatedPaths);
        localStorage.setItem('customPaths', JSON.stringify(updatedPaths));
    };

    const loadFiles = async () => {
        try {
            const response = await fetch(`/api/list?type=${activeTab}`);
            const data = await response.json();
            if (data.success) {
                setFiles(data.data || []);
            }
        } catch (error) {
            console.error('Error loading files:', error);
        }
    };

    const loadStats = async () => {
        try {
            const response = await fetch(`/api/stats?type=${activeTab}`);
            const data = await response.json();
            if (data.success) {
                setStats(data.data);
            }
        } catch (error) {
            console.error('Error loading stats:', error);
        }
    };

    const handleFileSelect = (event) => {
        const file = event.target.files[0];
        if (!file) return;

        setSelectedFile(file);
        
        // Generate preview for images
        if (file.type.startsWith('image/')) {
            const reader = new FileReader();
            reader.onloadend = () => {
                setPreviewUrl(reader.result);
            };
            reader.readAsDataURL(file);
        } else {
            setPreviewUrl(null);
        }
    };

    const handleUpload = async (event) => {
        event.preventDefault();
        if (!selectedFile) return;

        // Ensure we have a valid path
        const finalPath = uploadPath || selectedFile.name;

        const formData = new FormData();
        formData.append('file', selectedFile);
        
        // Add conversion type if selected
        if (conversionType !== 'none') {
            formData.append('conversion', conversionType);
        }

        const xhr = new XMLHttpRequest();
        xhr.open('POST', `/api/files?type=${activeTab}&path=${encodeURIComponent(finalPath)}`);

        xhr.upload.onprogress = (event) => {
            if (event.lengthComputable) {
                const progress = (event.loaded / event.total) * 100;
                setUploadProgress(prev => ({
                    ...prev,
                    [selectedFile.name]: progress
                }));
            }
        };

        xhr.onload = async () => {
            if (xhr.status === 200) {
                const response = JSON.parse(xhr.responseText);
                if (response.success) {
                    loadFiles();
                    loadStats();
                    fileInputRef.current.value = '';
                    setUploadPath('');
                    setSelectedFile(null);
                    setPreviewUrl(null);
                    setUploadProgress(prev => {
                        const newProgress = { ...prev };
                        delete newProgress[selectedFile.name];
                        return newProgress;
                    });
                }
            }
        };

        xhr.send(formData);
    };

    const handleDelete = async (path) => {
        if (!confirm('Are you sure you want to delete this file?')) return;

        try {
            const response = await fetch(`/api/files?type=${activeTab}&path=${path}`, {
                method: 'DELETE',
            });
            const data = await response.json();
            if (data.success) {
                loadFiles();
                loadStats();
            }
        } catch (error) {
            console.error('Error deleting file:', error);
        }
    };

    const formatSize = (bytes) => {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    };

    const handleRefresh = async () => {
        setIsRefreshing(true);
        await Promise.all([loadFiles(), loadStats()]);
        setIsRefreshing(false);
    };

    const isImage = (path) => {
        const imageExtensions = ['.jpg', '.jpeg', '.png', '.gif', '.bmp', '.webp'];
        return imageExtensions.some(ext => path.toLowerCase().endsWith(ext));
    };

    const loadExplorerContents = async (path) => {
        setExplorerLoading(true);
        try {
            const response = await fetch(`/api/explore?path=${encodeURIComponent(path)}`);
            const data = await response.json();
            if (data.success) {
                setExplorerItems(data.data.items);
                setExplorerPath(data.data.path);
            }
        } catch (error) {
            console.error('Error loading directory contents:', error);
        }
        setExplorerLoading(false);
    };

    const handleMount = async (path) => {
        try {
            const response = await fetch(`/api/mount?path=${encodeURIComponent(path)}`, {
                method: 'POST'
            });
            const data = await response.json();
            if (data.success) {
                setCustomPaths(prev => [...prev, path]);
                localStorage.setItem('customPaths', JSON.stringify([...customPaths, path]));
                setShowExplorer(false);
            }
        } catch (error) {
            console.error('Error mounting directory:', error);
        }
    };

    const formatDate = (dateString) => {
        return new Date(dateString).toLocaleString();
    };

    return (
        <div className="container">
            <h1>Virtual Disk Interface</h1>
            
            <div className="tabs">
                <button 
                    className={activeTab === 'disk' ? 'active' : ''} 
                    onClick={() => setActiveTab('disk')}
                >
                    Actual Drive
                </button>
                <button 
                    className={activeTab === 'temp' ? 'active' : ''} 
                    onClick={() => setActiveTab('temp')}
                >
                    Temp
                </button>
                <button 
                    className={activeTab === 'memory' ? 'active' : ''} 
                    onClick={() => setActiveTab('memory')}
                >
                    In-Memory
                </button>
                <button 
                    className={activeTab === 'mmap' ? 'active' : ''} 
                    onClick={() => setActiveTab('mmap')}
                >
                    MMapped
                </button>
                <button 
                    className="refresh-btn"
                    onClick={handleRefresh}
                    disabled={isRefreshing}
                >
                    {isRefreshing ? 'Refreshing...' : '🔄 Refresh'}
                </button>
            </div>

            {stats && (
                <div className="stats-grid">
                    <div className="stat-item">
                        <h3>Total Size</h3>
                        <p>{formatSize(stats.totalSize)}</p>
                    </div>
                    <div className="stat-item">
                        <h3>Used Space</h3>
                        <p>{formatSize(stats.usedSpace)}</p>
                    </div>
                    <div className="stat-item">
                        <h3>Free Space</h3>
                        <p>{formatSize(stats.freeSpace)}</p>
                    </div>
                    <div className="stat-item">
                        <h3>File Count</h3>
                        <p>{stats.fileCount}</p>
                    </div>
                </div>
            )}

            <div className="custom-paths">
                <h2>Custom Paths</h2>
                <div className="custom-paths-actions">
                    <button 
                        className="explore-btn"
                        onClick={() => {
                            setShowExplorer(true);
                            loadExplorerContents('/');
                        }}
                    >
                        📂 Browse Folders
                    </button>
                </div>
                <div className="custom-paths-list">
                    {customPaths.map((path, index) => (
                        <div key={index} className="custom-path-item">
                            <span>{path}</span>
                            <div className="path-actions">
                                <button onClick={() => removeCustomPath(path)}>Remove</button>
                            </div>
                        </div>
                    ))}
                </div>

                {showExplorer && (
                    <div className="explorer-overlay">
                        <div className="explorer-modal">
                            <div className="explorer-header">
                                <h3>Folder Explorer</h3>
                                <button onClick={() => setShowExplorer(false)}>✕</button>
                            </div>
                            <div className="explorer-path">
                                <button 
                                    onClick={() => loadExplorerContents(explorerPath.split('/').slice(0, -1).join('/') || '/')}
                                    disabled={explorerPath === '/'}
                                >
                                    ⬆️ Up
                                </button>
                                <span>{explorerPath}</span>
                            </div>
                            <div className="explorer-content">
                                {explorerLoading ? (
                                    <div className="explorer-loading">Loading...</div>
                                ) : (
                                    <div className="explorer-items">
                                        {explorerItems.map((item, index) => (
                                            <div 
                                                key={index} 
                                                className={`explorer-item ${item.isDir ? 'is-directory' : ''}`}
                                                onClick={() => item.isDir && loadExplorerContents(item.path)}
                                            >
                                                <span className="item-icon">
                                                    {item.isDir ? '📁' : '📄'}
                                                </span>
                                                <div className="item-details">
                                                    <span className="item-name">{item.name}</span>
                                                    <span className="item-info">
                                                        {item.isDir ? (
                                                            'Directory'
                                                        ) : (
                                                            formatSize(item.size)
                                                        )}
                                                        {' • '}
                                                        {formatDate(item.modified)}
                                                    </span>
                                                </div>
                                                {item.isDir && (
                                                    <button 
                                                        className="mount-btn"
                                                        onClick={(e) => {
                                                            e.stopPropagation();
                                                            handleMount(item.path);
                                                        }}
                                                    >
                                                        Mount
                                                    </button>
                                                )}
                                            </div>
                                        ))}
                                    </div>
                                )}
                            </div>
                        </div>
                    </div>
                )}
            </div>

            <div className="upload-form">
                <h2>Upload File</h2>
                <form onSubmit={handleUpload}>
                    <input
                        type="text"
                        placeholder="File path (e.g., folder/file.txt)"
                        value={uploadPath}
                        onChange={(e) => setUploadPath(e.target.value)}
                    />
                    <input
                        type="file"
                        ref={fileInputRef}
                        onChange={handleFileSelect}
                    />
                    <select
                        value={conversionType}
                        onChange={(e) => setConversionType(e.target.value)}
                        className="conversion-select"
                    >
                        <option value="none">No Conversion</option>
                        <option value="compress">Compress</option>
                        <option value="image-jpg">Convert to JPG</option>
                        <option value="image-png">Convert to PNG</option>
                        <option value="image-webp">Convert to WebP</option>
                    </select>
                    <button type="submit">Upload</button>
                </form>
                
                {previewUrl && (
                    <div className="preview-container">
                        <h3>Preview</h3>
                        <img src={previewUrl} alt="Upload preview" className="upload-preview" />
                    </div>
                )}
                
                {Object.entries(uploadProgress).map(([filename, progress]) => (
                    <div key={filename} className="progress-bar-container">
                        <div className="progress-label">{filename}: {Math.round(progress)}%</div>
                        <div className="progress-bar">
                            <div 
                                className="progress-fill"
                                style={{ width: `${progress}%` }}
                            />
                        </div>
                    </div>
                ))}
            </div>

            <div className="files-list">
                <h2>Files</h2>
                <div className="files-grid">
                    {files.map((file, index) => (
                        <div key={index} className="file-card">
                            {isImage(file.path) ? (
                                <img 
                                    src={`/api/files?type=${activeTab}&path=${file.path}`}
                                    alt={file.path}
                                    className="file-preview"
                                />
                            ) : (
                                <div className="file-icon">📄</div>
                            )}
                            <div className="file-info">
                                <div className="file-path">{file.path}</div>
                                <div className="file-size">{formatSize(file.size)}</div>
                                <button 
                                    onClick={() => handleDelete(file.path)}
                                    className="delete-btn"
                                >
                                    Delete
                                </button>
                            </div>
                        </div>
                    ))}
                </div>
            </div>
        </div>
    );
}

export default App;

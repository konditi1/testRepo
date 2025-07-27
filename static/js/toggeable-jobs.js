// Toggleable Jobs JavaScript
class ToggleableJobs {
    constructor() {
        this.currentSection = 'browse';
        this.isLoading = false;
        this.cache = new Map();
        
        this.init();
    }

    init() {
        this.createJobsPopupHTML();
        this.bindEvents();
    }

    createJobsPopupHTML() {
        const jobsOverlay = document.createElement('div');
        jobsOverlay.className = 'jobs-overlay';
        jobsOverlay.id = 'jobsOverlay';

        const jobsPopup = document.createElement('div');
        jobsPopup.className = 'jobs-popup';
        jobsPopup.id = 'jobsPopup';

        jobsPopup.innerHTML = `
            <div class="jobs-popup-header">
                <div class="jobs-popup-header-left">
                    <div class="jobs-popup-icon">
                        <i class="fas fa-briefcase"></i>
                    </div>
                    <h3 class="jobs-popup-title">M&E Job Opportunities</h3>
                </div>
                <button class="jobs-popup-close" id="jobsPopupClose" title="Close jobs">
                    <i class="fas fa-times"></i>
                </button>
            </div>
            
            <div class="jobs-popup-nav">
                <button class="jobs-nav-item active" data-section="browse">
                    <i class="fas fa-search"></i>
                    Browse Jobs
                </button>
                <button class="jobs-nav-item" data-section="post">
                    <i class="fas fa-plus"></i>
                    Post Job
                </button>
                <button class="jobs-nav-item" data-section="my-jobs">
                    <i class="fas fa-list"></i>
                    My Jobs
                </button>
                <button class="jobs-nav-item" data-section="my-applications">
                    <i class="fas fa-paper-plane"></i>
                    My Applications
                </button>
            </div>
            
            <div class="jobs-popup-content">
                <!-- Browse Jobs Section -->
                <div class="jobs-content-section active" id="browseJobsSection">
                    <div class="jobs-loading">
                        <i class="fas fa-spinner fa-spin"></i>
                        <span>Loading jobs...</span>
                    </div>
                </div>
                
                <!-- Post Job Section -->
                <div class="jobs-content-section" id="postJobSection">
                    <form class="popup-job-form" id="postJobForm">
                        <div class="popup-form-group">
                            <label for="jobTitle">Job Title *</label>
                            <input type="text" id="jobTitle" name="title" required placeholder="e.g., Senior M&E Officer">
                        </div>
                        
                        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 16px;">
                            <div class="popup-form-group">
                                <label for="jobLocation">Location *</label>
                                <input type="text" id="jobLocation" name="location" required placeholder="e.g., Nairobi, Kenya">
                            </div>
                            <div class="popup-form-group">
                                <label for="jobType">Employment Type *</label>
                                <select id="jobType" name="employment_type" required>
                                    <option value="">Select Type</option>
                                    <option value="full_time">Full Time</option>
                                    <option value="part_time">Part Time</option>
                                    <option value="contract">Contract</option>
                                    <option value="internship">Internship</option>
                                    <option value="volunteer">Volunteer</option>
                                </select>
                            </div>
                        </div>
                        
                        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 16px;">
                            <div class="popup-form-group">
                                <label for="jobSalary">Salary Range</label>
                                <input type="text" id="jobSalary" name="salary_range" placeholder="e.g., $50,000 - $70,000">
                            </div>
                            <div class="popup-form-group">
                                <label for="jobDeadline">Application Deadline *</label>
                                <input type="date" id="jobDeadline" name="application_deadline" required>
                            </div>
                        </div>
                        
                        <div class="popup-form-group">
                            <label for="jobDescription">Job Description *</label>
                            <textarea id="jobDescription" name="description" required 
                                      placeholder="Describe the role, organization background, and key objectives..."></textarea>
                        </div>
                        
                        <div class="popup-form-group">
                            <label for="jobResponsibilities">Key Responsibilities</label>
                            <textarea id="jobResponsibilities" name="responsibilities"
                                      placeholder="List the main duties and responsibilities..."></textarea>
                        </div>
                        
                        <div class="popup-form-group">
                            <label for="jobRequirements">Requirements & Qualifications</label>
                            <textarea id="jobRequirements" name="requirements"
                                      placeholder="Education, experience, skills, and other requirements..."></textarea>
                        </div>
                        
                        <button type="submit" class="popup-job-btn">
                            <i class="fas fa-upload"></i>
                            Post Job
                        </button>
                    </form>
                </div>
                
                <!-- My Jobs Section -->
                <div class="jobs-content-section" id="myJobsSection">
                    <div class="jobs-loading">
                        <i class="fas fa-spinner fa-spin"></i>
                        <span>Loading your jobs...</span>
                    </div>
                </div>
                
                <!-- My Applications Section -->
                <div class="jobs-content-section" id="myApplicationsSection">
                    <div class="jobs-loading">
                        <i class="fas fa-spinner fa-spin"></i>
                        <span>Loading your applications...</span>
                    </div>
                </div>
            </div>
        `;

        document.body.appendChild(jobsOverlay);
        document.body.appendChild(jobsPopup);
    }

    bindEvents() {
        // Jobs section links in sidebar
        document.addEventListener('click', (e) => {
            const jobsLink = e.target.closest('.btn-jobs-link');
            if (jobsLink) {
                e.preventDefault();
                const href = jobsLink.href;
                this.handleJobsLinkClick(href);
            }
        });

        // Jobs navigation in dashboard
        document.addEventListener('click', (e) => {
            const createJobBtn = e.target.closest('.btn-create-job-modern, .btn-create-modern[href="/create-job"]');
            if (createJobBtn) {
                e.preventDefault();
                this.openJobs('post');
            }

            const myJobsBtn = e.target.closest('.btn-my-jobs-modern, .btn-create-modern[href="/my-jobs"]');
            if (myJobsBtn) {
                e.preventDefault();
                this.openJobs('my-jobs');
            }

            const myAppsBtn = e.target.closest('.btn-my-apps-modern, .btn-create-modern[href="/my-applications"]');
            if (myAppsBtn) {
                e.preventDefault();
                this.openJobs('my-applications');
            }
        });

        // Close jobs popup
        document.addEventListener('click', (e) => {
            if (e.target.matches('#jobsPopupClose') || e.target.closest('#jobsPopupClose')) {
                this.closeJobs();
            }
            
            // Close when clicking overlay
            if (e.target.matches('#jobsOverlay')) {
                this.closeJobs();
            }
        });

        // Navigation tabs
        document.addEventListener('click', (e) => {
            const navItem = e.target.closest('.jobs-nav-item');
            if (navItem) {
                const section = navItem.getAttribute('data-section');
                this.switchSection(section);
            }
        });

        // Form submissions
        document.addEventListener('submit', (e) => {
            if (e.target.matches('#postJobForm')) {
                e.preventDefault();
                this.handleJobPost(e.target);
            }
        });

        // Job actions within popup
        document.addEventListener('click', (e) => {
            const applyBtn = e.target.closest('.popup-apply-btn');
            if (applyBtn) {
                e.preventDefault();
                const jobId = applyBtn.getAttribute('data-job-id');
                this.handleJobApplication(jobId);
            }

            const viewBtn = e.target.closest('.popup-view-btn');
            if (viewBtn) {
                e.preventDefault();
                const jobId = viewBtn.getAttribute('data-job-id');
                this.handleJobView(jobId);
            }
        });

        // ESC key to close
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && this.isJobsOpen()) {
                this.closeJobs();
            }
        });
    }

    handleJobsLinkClick(href) {
        const url = new URL(href, window.location.origin);
        const pathname = url.pathname;

        switch (pathname) {
            case '/jobs':
                this.openJobs('browse');
                break;
            case '/create-job':
                this.openJobs('post');
                break;
            case '/my-jobs':
                this.openJobs('my-jobs');
                break;
            case '/my-applications':
                this.openJobs('my-applications');
                break;
            default:
                // For other job-related links, open in new tab/window
                window.open(href, '_blank');
        }
    }

    async openJobs(section = 'browse') {
        this.currentSection = section;
        
        // Show popup
        this.showJobsPopup();
        
        // Switch to the requested section
        this.switchSection(section);
        
        // Load content for
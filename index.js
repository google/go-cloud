const Git = require('nodegit');
const GitHubApi = require('github');
const uniqBy = require('lodash.uniqby');

const github = new GitHubApi();

const attemptOpen = local => Git.Repository.open(local);

const promises = [
	github.search.repos({q: `meltdown`, sort: 'updated', per_page: 100}),
	github.search.repos({q: `spectre`, sort: 'updated', per_page: 100})
];

const meltdownRepos = [];
Promise.all(promises)
	.then(repos => {
		repos.forEach(data => {
			data.data.items.forEach(elem => {
				if (isRelated(elem)) {
					meltdownRepos.push(elem);
				}
			});
		});
		const uniqRepos = uniqBy(meltdownRepos, 'full_name');
		uniqRepos.forEach(elem => {
			console.log(`Cloning: repos/${elem.owner.login}/${elem.name}`);
			Git.Clone(elem.clone_url, `repos/${elem.owner.login}/${elem.name}`)
				.then(() => {
					console.log(`Cloned: repos/${elem.owner.login}/${elem.name}`);
				})
				.catch(err => {
					if (err.errno === -4) {
						updateRepo(elem);
					} else {
						console.log(err);
					}
				});
		});
	})
	.catch(err => {
		console.log(err);
	});

function updateRepo(repoObj) {
	console.log(`Already cloned, updating: repos/${repoObj.owner.login}/${repoObj.name}`);
	attemptOpen(`repos/${repoObj.owner.login}/${repoObj.name}`)
		.then(async repo => {
			let commit, oldDate;
			try {
				commit = await repo.getMasterCommit();
				oldDate = Math.floor(commit.date());
			} catch (err) {
				oldDate = null;
			}
			repo
				.fetchAll()
				.then(async () => {
					let newCommit, newDate;
					try {
						newCommit = await repo.getMasterCommit();
						newDate = Math.floor(newCommit.date());
					} catch (err) {
						newDate = null;
					}

					if (newDate && oldDate && newDate !== oldDate) {
						console.log(`repos/${repoObj.owner.login}/${repoObj.name} has changes.`);
					} else if (!newDate || !oldDate) {
						console.log(`repos/${repoObj.owner.login}/${repoObj.name} might have changes.`)
					}
					repo
						.mergeBranches('master', 'origin/master')
						.catch(err => {
							if (err.errno !== -3) {
								console.error(err);
								console.log(`repos/${repoObj.owner.login}/${repoObj.name} errored.`);
							}
						});
				})
				.catch(err => {
					console.error(err);
					console.log(`repos/${repoObj.owner.login}/${repoObj.name} errored.`);
				});
		})
		.catch(err => {
			console.log(err);
			console.log(`repos/${repoObj.owner.login}/${repoObj.name} errored.`);
		});
}

function isRelated(elem) {
	if (elem.description) {
		elem.description = `${elem.full_name} - ${elem.description}`;
	} else {
		elem.description = elem.full_name;
	}
	if (elem.description.search(/(cve|exploit|attack|poc|example|bug|patch|check|[ae]ffected|vuln[erability]*)/igm) >= 0) {
		return true;
	}
}
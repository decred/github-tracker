# Github Tracker
[![Build Status](https://github.com/decred/github-tracker/workflows/Build%20and%20Test/badge.svg)](https://github.com/github-tracker/politeia/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)

** Github tracker fetches user development stats across an organization **
Github tracker allows for periodic updates to fetch all information concerning
pullrequests, commits, and reviews for a given organization.

To give a real-world example, it will be used in the following way for Decred:

- The Decred Contractor Management System (CMS) will periodically (every month) request
for code stats be updated for all repos that Decred controls.  By doing so,
it will update and populate a relational database for all pull requests, reviews,
and commits that have been completed or updated in that time period.  

Then once all the information is in an easily queryble database, CMS can ask
for code stats for any given user.  These stats are going to be used to determine
regular levels of output for the multitude of remote workers.  These stats will
be shown to administrators that will approve invoices, but also other contractors.
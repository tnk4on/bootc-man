# .copr/Makefile - Used by COPR SCM build method
# Ref: https://docs.pagure.org/copr.copr/user_documentation.html#scm
# Note: COPR runs "cd .copr && make -f Makefile srpm", so all paths
#       must be relative to this directory or use absolute paths.

TOPDIR := $(dir $(realpath $(lastword $(MAKEFILE_LIST))))..
SPEC_FILE := $(TOPDIR)/rpm/bootc-man.spec

srpm:
	dnf install -y git rpm-build rpmdevtools
	spectool -g -C $(TOPDIR) $(SPEC_FILE)
	rpmbuild -bs --define "_sourcedir $(TOPDIR)" \
		--define "_srcrpmdir $(outdir)" \
		$(SPEC_FILE)

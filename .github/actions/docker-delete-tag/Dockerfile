FROM alpine 

LABEL "com.github.actions.name"="Docker Delete Tag"
LABEL "com.github.actions.description"="Deletes docker tag on docker hub"
LABEL "com.github.actions.icon"="delete"
LABEL "com.github.actions.color"="red"

RUN apk --no-cache add curl jq bash

ADD entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]